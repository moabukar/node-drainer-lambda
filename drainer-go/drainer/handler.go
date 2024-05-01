package main

import (
	"context"
	"encoding/base64"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	kubeFilePath        = "/tmp/kubeconfig"
	clusterNameEnv      = "CLUSTER_NAME"
	kubeConfigBucketEnv = "KUBE_CONFIG_BUCKET"
	kubeConfigObjectEnv = "KUBE_CONFIG_OBJECT"
	awsRegionEnv        = "AWS_REGION"
)

var (
	sess          = session.Must(session.NewSession())
	eksClient     = eks.New(sess)
	ec2Client     = ec2.New(sess)
	asgClient     = autoscaling.New(sess)
	s3Client      = s3.New(sess)
	stsClient     = sts.New(sess)
	kubeClientSet *kubernetes.Clientset
)

func handler(ctx context.Context, event map[string]interface{}) error {
	// Ensure the kubeconfig exists
	ensureKubeConfig()

	// Process event to get instance and node details
	details := event["detail"].(map[string]interface{})
	lifecycleHookName := details["LifecycleHookName"].(string)
	autoScalingGroupName := details["AutoScalingGroupName"].(string)
	instanceID := details["EC2InstanceId"].(string)

	// Get node name from EC2 Instance
	nodeName, err := getNodeNameFromInstanceID(instanceID)
	if err != nil {
		log.Fatal("Failed to get node name:", err)
	}

	// Cordon and Drain Node
	if err := cordonAndDrainNode(nodeName); err != nil {
		log.Fatal("Failed to cordon and drain node:", err)
	}

	// Complete Lifecycle Hook
	return completeLifecycleHook(lifecycleHookName, autoScalingGroupName, instanceID, "CONTINUE")
}

func ensureKubeConfig() {
	if _, err := os.Stat(kubeFilePath); os.IsNotExist(err) {
		if os.Getenv(kubeConfigBucketEnv) != "" {
			log.Println("Downloading kubeconfig from S3...")
			err := downloadKubeConfig()
			if err != nil {
				log.Fatal("Failed to download kubeconfig:", err)
			}
		} else {
			log.Println("Generating kubeconfig...")
			err := generateKubeConfig()
			if err != nil {
				log.Fatal("Failed to generate kubeconfig:", err)
			}
		}
	}

	var err error
	kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeFilePath)
	if err != nil {
		log.Fatal("Failed to build kubeconfig:", err)
	}

	kubeClientSet, err = kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatal("Failed to create Kubernetes client set:", err)
	}
}

func downloadKubeConfig() error {
	bucket := os.Getenv(kubeConfigBucketEnv)
	object := os.Getenv(kubeConfigObjectEnv)
	return s3Client.DownloadFile(bucket, object, kubeFilePath)
}

func generateKubeConfig() error {
	clusterName := os.Getenv(clusterNameEnv)
	region := os.Getenv(awsRegionEnv)
	out, err := eksClient.DescribeCluster(&eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	})
	if err != nil {
		return err
	}

	cluster := out.Cluster
	certificate := cluster.CertificateAuthority.Data
	endpoint := cluster.Endpoint

	kubeConfig := map[string]interface{}{
		"apiVersion": "v1",
		"clusters": []interface{}{
			map[string]interface{}{
				"cluster": map[string]interface{}{
					"server":                     endpoint,
					"certificate-authority-data": certificate,
				},
				"name": "k8s",
			},
		},
		"contexts": []interface{}{
			map[string]interface{}{
				"context": map[string]interface{}{
					"cluster": "k8s",
					"user":    "aws",
				},
				"name": "aws",
			},
		},
		"current-context": "aws",
		"kind":            "Config",
		"users": []interface{}{
			map[string]interface{}{
				"name": "aws",
				"user": map[string]interface{}{
					"token": generateAuthToken(clusterName, region),
				},
			},
		},
	}

	data, err := yaml.Marshal(kubeConfig)
	if err != nil {
		return err
	}

	return os.WriteFile(kubeFilePath, data, 0644)
}

func generateAuthToken(clusterName, region string) string {
	service := "sts"
	stsReq, _ := stsClient.GetCallerIdentityRequest(&sts.GetCallerIdentityInput{})
	stsReq.HTTPRequest.Header.Add("x-k8s-aws-id", clusterName)
	stsReq.Sign()

	authToken := base64.StdEncoding.EncodeToString([]byte(stsReq.HTTPRequest.URL.String()))
	return "k8s-aws-v1." + strings.TrimRight(authToken, "=")
}
