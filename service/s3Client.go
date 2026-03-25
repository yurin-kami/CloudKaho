package service

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appconfig "github.com/yurin-kami/CloudKaho/config"
)

func NewS3Client(ctx context.Context) (*s3.Client, error) {
	s3Config := appconfig.Get().S3
	loadOptions := []func(*awscfg.LoadOptions) error{}
	if s3Config.Region != "" {
		loadOptions = append(loadOptions, awscfg.WithRegion(s3Config.Region))
	}

	cfg, err := awscfg.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if s3Config.Endpoint != "" {
			o.BaseEndpoint = aws.String(s3Config.Endpoint)
		}
		o.UsePathStyle = s3Config.UsePathStyle
	}), nil
}
