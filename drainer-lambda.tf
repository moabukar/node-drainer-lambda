resource "aws_lambda_function" "node_drainer" {
  filename      = "drainer.zip"
  #filename      = "drainer-go.zip" ## For golang version
  function_name = "${var.name_prefix}-node-drainer-function"
  role          = aws_iam_role.node_drainer.arn
  handler       = "handler.lambda_handler"
  # handler       = "main" ## compiled handler for Golang
  memory_size   = "256"
  timeout       = "300"

  source_code_hash = filebase64sha256("drainer.zip")
  # source_code_hash = filebase64sha256("drainer-go.zip")

  runtime = "python3.8"
  # runtime = "provided.al2023" ## used for custom runtime or Golang

  environment {
    variables = {
      CLUSTER_NAME = var.cluster_name
      REGION       = var.region
    }
  }

  vpc_config {
    subnet_ids         = var.subnets
    security_group_ids = var.security_group_ids
  }

  tags = var.tags
}

resource "aws_lambda_permission" "allow_invoke_function_1" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.node_drainer.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.node_drainer.arn
}

resource "aws_lambda_permission" "allow_invoke_function_2" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.node_drainer.function_name
  principal     = "events.amazonaws.com"
}

resource "aws_cloudwatch_event_rule" "node_drainer" {
  name        = "${var.name_prefix}-node-drainer-event-rule"
  description = "EKS node drainer Event Rule"

  event_pattern = <<PATTERN
{
  "detail-type": [
    "EC2 Instance-terminate Lifecycle Action"
  ],
  "source": [
    "aws.autoscaling"
  ],
  "detail": {
    "AutoScalingGroupName": [
      "${var.auto_scaling_group_name}"
    ]
  }
}
PATTERN

  tags = var.tags
}

resource "aws_cloudwatch_event_target" "node_drainer" {
  rule = aws_cloudwatch_event_rule.node_drainer.name
  arn  = aws_lambda_function.node_drainer.arn
}


