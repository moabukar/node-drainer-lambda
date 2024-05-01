package drainer

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

func getNodeNameFromInstanceID(instanceID string) (string, error) {
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
	}
	result, err := ec2Client.DescribeInstances(input)
	if err != nil {
		return "", err
	}
	if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		return "", fmt.Errorf("no instances found for ID: %s", instanceID)
	}
	return *result.Reservations[0].Instances[0].PrivateDnsName, nil
}

func cordonAndDrainNode(nodeName string) error {
	// Patch node to mark it as unschedulable (cordon)
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := kubeClientSet.CoreV1().Nodes().Patch(context.TODO(), nodeName,
			types.StrategicMergePatchType,
			[]byte(`{"spec":{"unschedulable":true}}`),
			metav1.PatchOptions{})
		return err
	}); err != nil {
		return err
	}

	// Get all pods on the node and attempt to evict them
	pods, err := kubeClientSet.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return err
	}

	for _, pod := range pods.Items {
		if !isEvictable(pod) {
			continue
		}
		eviction := &v1beta1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}
		if err := kubeClientSet.CoreV1().Pods(pod.Namespace).Evict(context.TODO(), eviction); err != nil {
			log.Printf("Error evicting pod %s: %v", pod.Name, err)
		}
	}

	return nil
}

func isEvictable(pod v1.Pod) bool {
	// Implement logic to determine if a pod can be evicted, similar to the Python version
	return true
}

func completeLifecycleHook(lifecycleHookName, autoScalingGroupName, instanceID, result string) error {
	_, err := asgClient.CompleteLifecycleAction(&autoscaling.CompleteLifecycleActionInput{
		LifecycleHookName:     aws.String(lifecycleHookName),
		AutoScalingGroupName:  aws.String(autoScalingGroupName),
		LifecycleActionResult: aws.String(result),
		InstanceId:            aws.String(instanceID),
	})
	return err
}

func abandonLifecycleAction(lifecycleHookName, autoScalingGroupName, instanceID string) {
	asgClient.CompleteLifecycleAction(&autoscaling.CompleteLifecycleActionInput{
		LifecycleHookName:     aws.String(lifecycleHookName),
		AutoScalingGroupName:  aws.String(autoScalingGroupName),
		LifecycleActionResult: aws.String("ABANDON"),
		InstanceId:            aws.String(instanceID),
	})
}
