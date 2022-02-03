package main

import (
	"context"
	"os"

	"github.com/cloudquery/cloudquery/cmd"
	"github.com/cloudquery/cloudquery/deploy"
)

func main() {
	if env := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); env != "" {
		// lambda.Start(deploy.LambdaHandler)
		deploy.LambdaHandler(context.TODO(), deploy.Request{})
		return
	}
	cmd.Execute()
}
