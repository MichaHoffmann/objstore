// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v2"

	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/providers/azure"
	"github.com/thanos-io/objstore/providers/bos"
	"github.com/thanos-io/objstore/providers/cos"
	"github.com/thanos-io/objstore/providers/filesystem"
	"github.com/thanos-io/objstore/providers/gcs"
	"github.com/thanos-io/objstore/providers/oci"
	"github.com/thanos-io/objstore/providers/oss"
	"github.com/thanos-io/objstore/providers/s3"
	"github.com/thanos-io/objstore/providers/swift"
)

type ObjProvider string

const (
	FILESYSTEM ObjProvider = "FILESYSTEM"
	GCS        ObjProvider = "GCS"
	S3         ObjProvider = "S3"
	AZURE      ObjProvider = "AZURE"
	SWIFT      ObjProvider = "SWIFT"
	COS        ObjProvider = "COS"
	ALIYUNOSS  ObjProvider = "ALIYUNOSS"
	BOS        ObjProvider = "BOS"
	OCI        ObjProvider = "OCI"
)

type BucketConfig struct {
	Type                 ObjProvider                `yaml:"type"`
	Config               interface{}                `yaml:"config"`
	Prefix               string                     `yaml:"prefix" default:""`
	ClientSideEncryption ClientSideEncryptionConfig `yaml:"client_side_encryption"`
}

type ClientSideEncryptionConfig struct {
	Enabled   bool   `yaml:"enabled"`
	KeyBase64 string `yaml:"key_base64"`
}

// NewBucket initializes and returns new object storage clients.
// NOTE: confContentYaml can contain secrets.
func NewBucket(logger log.Logger, confContentYaml []byte, reg prometheus.Registerer, component string) (objstore.InstrumentedBucket, error) {
	level.Info(logger).Log("msg", "loading bucket configuration")
	bucketConf := &BucketConfig{}
	if err := yaml.UnmarshalStrict(confContentYaml, bucketConf); err != nil {
		return nil, errors.Wrap(err, "parsing config YAML file")
	}

	config, err := yaml.Marshal(bucketConf.Config)
	if err != nil {
		return nil, errors.Wrap(err, "marshal content of bucket configuration")
	}

	var bucket objstore.Bucket
	switch strings.ToUpper(string(bucketConf.Type)) {
	case string(GCS):
		bucket, err = gcs.NewBucket(context.Background(), logger, config, component)
	case string(S3):
		bucket, err = s3.NewBucket(logger, config, component)
	case string(AZURE):
		bucket, err = azure.NewBucket(logger, config, component)
	case string(SWIFT):
		bucket, err = swift.NewContainer(logger, config)
	case string(COS):
		bucket, err = cos.NewBucket(logger, config, component)
	case string(ALIYUNOSS):
		bucket, err = oss.NewBucket(logger, config, component)
	case string(FILESYSTEM):
		bucket, err = filesystem.NewBucketFromConfig(config)
	case string(BOS):
		bucket, err = bos.NewBucket(logger, config, component)
	case string(OCI):
		bucket, err = oci.NewBucket(logger, config)
	default:
		return nil, errors.Errorf("bucket with type %s is not supported", bucketConf.Type)
	}
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("create %s client", bucketConf.Type))
	}

	if bucketConf.ClientSideEncryption.Enabled {
		key, err := base64.RawStdEncoding.DecodeString(bucketConf.ClientSideEncryption.KeyBase64)
		if err != nil {
			return nil, errors.Wrap(err, "unable to read base64 key")
		}
		if len(key) != 32 {
			return nil, errors.New("decoded key must have size 32")
		}
		bucket = objstore.BucketWithEncryption(bucket, key)
	}

	return objstore.NewTracingBucket(objstore.BucketWithMetrics(bucket.Name(), objstore.NewPrefixedBucket(bucket, bucketConf.Prefix), reg)), nil
}
