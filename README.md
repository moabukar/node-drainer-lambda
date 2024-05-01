# A lambda to drain nodes in K8s

- Drain pods in K8s from EKS worker nodes during autoscaling scale-in events. The code provides an AWS Lambda function that integrates as an Amazon EC2 Auto Scaling Lifecycle Hook. When called, the Lambda function calls the Kubernetes API to cordon and evict all evictable pods from the node being terminated. It will then wait until all pods have been evicted before the Auto Scaling group continues to terminate the EC2 instance.

## Deploying

```sh
make tf-plan-dev
make tf-apply-dev
```

If needed to create new zip, `make build-lambda`


## Autoscaling group lifecycle hook

```sh

resource "aws_autoscaling_group" "asg" {
  name                      = "terraform-test"
  ...

  initial_lifecycle_hook {
    name                 = "test"
    default_result       = "CONTINUE"
    heartbeat_timeout    = 180
    lifecycle_transition = "autoscaling:EC2_INSTANCE_TERMINATING"
  }
  ...
}
```

### K8s perms 

`kubectl apply -R -f terraform/node_drainer/k8s_rbac/`

### Test lambda drainer 

`aws autoscaling terminate-instance-in-auto-scaling-group --no-should-decrement-desired-capacity --instance-id <instance-id>`

## TODO
- [X] Create drainer in Python
- [X] Create drainer in Go
