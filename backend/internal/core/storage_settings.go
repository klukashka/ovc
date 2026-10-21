package core

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	varStorageBackend   = "OVC_STORAGE_BACKEND"
	varStorageEndpoint  = "OVC_STORAGE_ENDPOINT"
	varStorageRegion    = "OVC_STORAGE_REGION"
	varStorageBucket    = "OVC_STORAGE_BUCKET"
	varStorageAccessKey = "OVC_STORAGE_ACCESS_KEY"
	varStorageKeyValue  = "OVC_STORAGE_" + "SECRET_KEY"

	varDevStorageAccessKey = "OVC_DEV_STORAGE_ACCESS_KEY"
	varDevStorageKeyValue  = "OVC_DEV_STORAGE_" + "SECRET_KEY"
	varDevStoragePort      = "OVC_DEV_STORAGE_PORT"

	defaultDevStoragePort = "58333"
	testStorageRegion     = "us-east-1"
)

type StorageBackend string

const StorageBackendS3 StorageBackend = "s3"

var (
	bucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
	regionName = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	portNumber = regexp.MustCompile(`^[1-9][0-9]{0,4}$`)
)

type StorageSettings struct {
	Backend   StorageBackend
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey Secret
	SecretKey Secret
}

func storageVariableNames() []string {
	return []string{varStorageBackend, varStorageEndpoint, varStorageRegion, varStorageBucket, varStorageAccessKey, varStorageKeyValue}
}

func (s *Settings) loadStorageSettings(lookup Lookup) []error {
	var problems []error
	backend, err := oneOf(lookup, varStorageBackend, StorageBackendS3, StorageBackendS3)
	if err != nil {
		problems = append(problems, err)
	}
	endpoint, _ := lookup(varStorageEndpoint)
	if !validStorageEndpoint(endpoint) {
		problems = append(problems, &SettingsError{Variable: varStorageEndpoint, Problem: "is required: an https origin, or http only on a loopback host"})
	}
	region, _ := lookup(varStorageRegion)
	if !regionName.MatchString(region) {
		problems = append(problems, &SettingsError{Variable: varStorageRegion, Problem: "is required: lower-case letters, digits and hyphens"})
	}
	bucket, _ := lookup(varStorageBucket)
	if !bucketName.MatchString(bucket) {
		problems = append(problems, &SettingsError{Variable: varStorageBucket, Problem: "is required: 3 to 63 lower-case letters, digits and hyphens"})
	}
	accessKey, _ := lookup(varStorageAccessKey)
	if strings.TrimSpace(accessKey) == "" {
		problems = append(problems, &SettingsError{Variable: varStorageAccessKey, Problem: "is required"})
	}
	secretKey, _ := lookup(varStorageKeyValue)
	if strings.TrimSpace(secretKey) == "" {
		problems = append(problems, &SettingsError{Variable: varStorageKeyValue, Problem: "is required"})
	}
	s.Storage = StorageSettings{
		Backend: backend, Endpoint: strings.TrimRight(endpoint, "/"), Region: region, Bucket: bucket,
		AccessKey: NewSecret(accessKey), SecretKey: NewSecret(secretKey),
	}
	return problems
}

func validStorageEndpoint(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Trim(parsed.Path, "/") != "" {
		return false
	}
	switch parsed.Scheme {
	case "https":
		return true
	case "http":
		host := parsed.Hostname()
		return host == "127.0.0.1" || host == "::1" || host == "localhost"
	}
	return false
}

func testStorageFrom(lookup Lookup, bucket string) (StorageSettings, error) {
	accessKey, _ := lookup(varDevStorageAccessKey)
	secretKey, _ := lookup(varDevStorageKeyValue)
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return StorageSettings{}, &SettingsError{
			Variable: varDevStorageAccessKey,
			Problem:  "and " + varDevStorageKeyValue + " are required to run the storage tests; run them through tools/dev-env.sh",
		}
	}
	port, found := lookup(varDevStoragePort)
	if !found || port == "" {
		port = defaultDevStoragePort
	}
	if !portNumber.MatchString(port) {
		return StorageSettings{}, &SettingsError{Variable: varDevStoragePort, Problem: "must be a port number"}
	}
	return StorageSettings{
		Backend: StorageBackendS3, Endpoint: "http://127.0.0.1:" + port, Region: testStorageRegion, Bucket: bucket,
		AccessKey: NewSecret(accessKey), SecretKey: NewSecret(secretKey),
	}, nil
}
