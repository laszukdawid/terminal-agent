package cloud

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
)

func NewAwsConfig(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, opts...)
}

func NewDefaultAwsConfig(ctx context.Context) (aws.Config, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = 3
			})
		}),
	}

	return config.LoadDefaultConfig(ctx, opts...)
}

// NewAwsConfigWithSSO returns an aws.Config configured for use with AWS SSO credentials.
// The profileName parameter should match the name of the profile configured for SSO in your AWS credentials/config files.
func NewAwsConfigWithSSO(ctx context.Context, profileName string) (aws.Config, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithSharedConfigProfile(profileName),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = 3
			})
		}),
	}

	return config.LoadDefaultConfig(ctx, opts...)
}
