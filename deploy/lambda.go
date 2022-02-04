package deploy

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/cloudquery/cloudquery/pkg/client"
	"github.com/cloudquery/cloudquery/pkg/config"

	"github.com/spf13/viper"
)

type Request struct {
	TaskName string `json:"taskName"`
	HCL      string `json:"hcl"`
}

func LambdaHandler(ctx context.Context, req Request) (string, error) {

	getSecret("test")
	return TaskExecutor(ctx, req)
}

func TaskExecutor(ctx context.Context, req Request) (string, error) {
	dsn := os.Getenv("CQ_DSN")
	databasePasswordName := os.Getenv("CQ_DB_PASSWORD")
	dataDir, present := os.LookupEnv("CQ_DATA_DIR")
	if !present {
		dataDir = ".cq"
	}
	viper.Set("data-dir", dataDir)
	pluginDir, present := os.LookupEnv("CQ_PLUGIN_DIR")
	if !present {
		pluginDir = "."
	}
	viper.Set("plugin-dir", pluginDir)
	policyDir, present := os.LookupEnv("CQ_POLICY_DIR")
	if !present {
		policyDir = "."
	}
	viper.Set("policy-dir", policyDir)

	cfg, diags := config.NewParser(
		config.WithEnvironmentVariables(config.EnvVarPrefix, os.Environ()),
	).LoadConfigFromSource("config.hcl", []byte(req.HCL))
	if diags != nil {
		return "", fmt.Errorf("bad configuration: %s", diags)
	}

	// Override dsn env if set
	if dsn != "" {
		res := getSecret(databasePasswordName)
		fmt.Println("dsn", res)
		cfg.CloudQuery.Connection.DSN = res
	}

	completedMsg := fmt.Sprintf("Completed task %s", req.TaskName)
	switch req.TaskName {
	case "fetch":
		return completedMsg, Fetch(ctx, cfg)
	case "policy":
		return completedMsg, Policy(ctx, cfg)
	default:
		return fmt.Sprintf("Unknown task: %s", req.TaskName), fmt.Errorf("unknown task: %s", req.TaskName)
	}
}

// Fetch fetches resources from a cloud provider and saves them in the configured database
func Fetch(ctx context.Context, cfg *config.Config) error {
	c, err := client.New(ctx, func(c *client.Client) {
		c.Providers = cfg.CloudQuery.Providers
		c.PluginDirectory = cfg.CloudQuery.PluginDirectory
		c.PolicyDirectory = cfg.CloudQuery.PolicyDirectory
		c.DSN = cfg.CloudQuery.Connection.DSN
	})
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}
	defer c.Close()
	if err := c.DownloadProviders(ctx); err != nil {
		return err
	}
	if err := c.NormalizeResources(ctx, cfg.Providers); err != nil {
		return err
	}
	_, err = c.Fetch(ctx, client.FetchRequest{
		Providers: cfg.Providers,
	})
	if err != nil {
		return fmt.Errorf("error fetching resources: %w", err)
	}
	return nil
}

// Policy Runs a policy SQL statement and returns results
func Policy(ctx context.Context, cfg *config.Config) error {
	outputPath := "/tmp/"
	c, err := client.New(ctx, func(c *client.Client) {
		c.PluginDirectory = cfg.CloudQuery.PluginDirectory
		c.DSN = cfg.CloudQuery.Connection.DSN
		c.PolicyDirectory = cfg.CloudQuery.PolicyDirectory
	})
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}
	_, err = c.RunPolicies(ctx, &client.PoliciesRunRequest{
		Policies:  cfg.Policies,
		OutputDir: outputPath,
	})
	if err != nil {
		return fmt.Errorf("error running query: %s", err)
	}
	return nil
}

func getSecret(secretId string) string {

	svc := secretsmanager.New(session.New())

	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretId),
	}
	res, err := svc.GetSecretValue(input)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case secretsmanager.ErrCodeResourceNotFoundException:
				fmt.Println(secretsmanager.ErrCodeResourceNotFoundException, aerr.Error())
			case secretsmanager.ErrCodeInvalidParameterException:
				fmt.Println(secretsmanager.ErrCodeInvalidParameterException, aerr.Error())
			case secretsmanager.ErrCodeInvalidRequestException:
				fmt.Println(secretsmanager.ErrCodeInvalidRequestException, aerr.Error())
			case secretsmanager.ErrCodeDecryptionFailure:
				fmt.Println(secretsmanager.ErrCodeDecryptionFailure, aerr.Error())
			case secretsmanager.ErrCodeInternalServiceError:
				fmt.Println(secretsmanager.ErrCodeInternalServiceError, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		os.Exit(1)
	}

	return *res.SecretString
}
