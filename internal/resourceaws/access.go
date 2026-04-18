package resourceaws

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

const (
	UnavailableReasonUnsupportedResourceType   = "unsupported_resource_type"
	UnavailableReasonCrossAccountNotConfigured = "cross_account_not_configured"
	UnavailableReasonNoLogSource               = "no_log_source"
	UnavailableReasonNoRunningTasks            = "no_running_tasks"
	UnavailableReasonNoInstances               = "no_instances"
	UnavailableReasonResourceNotFound          = "resource_not_found"
	UnavailableReasonAWSUnavailable            = "aws_unavailable"
)

var awsUnavailableErrorSubstrings = []string{
	"failed to refresh cached credentials",
	"no ec2 imds role found",
	"no valid credential sources",
	"no credential providers",
	"failed to load credentials",
	"missing credentials",
}

var awsUnavailableErrorCodes = map[string]struct{}{
	"accessdenied":                {},
	"accessdeniedexception":       {},
	"accessdeniedfordependency":   {},
	"authfailure":                 {},
	"invalidclienttokenid":        {},
	"invalidsignatureexception":   {},
	"missingauthenticationtoken":  {},
	"optinrequired":               {},
	"signaturedoesnotmatch":       {},
	"unauthorizedoperation":       {},
	"unrecognizedclientexception": {},
}

type RequestContext struct {
	ControlAccountID string
	TargetAccountID  string
	Region           string
	ObserverRoleARN  string
}

type LookupContext struct {
	Region                 string `json:"region"`
	ControlAccountID       string `json:"controlAccountId"`
	TargetAccountID        string `json:"targetAccountId"`
	CrossAccount           bool   `json:"crossAccount"`
	ObserverRoleConfigured bool   `json:"observerRoleConfigured"`
}

type unavailableError struct{ reason string }

func (e unavailableError) Error() string { return e.reason }

func NewUnavailableError(reason string) error {
	return unavailableError{reason: reason}
}

func UnavailableReasonForError(err error) (string, bool) {
	var unavailable unavailableError
	if errors.As(err, &unavailable) {
		return unavailable.reason, true
	}
	if IsAWSUnavailableError(err) {
		return UnavailableReasonAWSUnavailable, true
	}
	return "", false
}

func IsAWSUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if _, ok := awsUnavailableErrorCodes[strings.ToLower(strings.TrimSpace(apiErr.ErrorCode()))]; ok {
			return true
		}
	}
	message := strings.ToLower(err.Error())
	for _, part := range awsUnavailableErrorSubstrings {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func Lookup(ctx RequestContext) LookupContext {
	controlAccountID := strings.TrimSpace(ctx.ControlAccountID)
	targetAccountID := strings.TrimSpace(ctx.TargetAccountID)
	return LookupContext{
		Region:                 strings.TrimSpace(ctx.Region),
		ControlAccountID:       controlAccountID,
		TargetAccountID:        targetAccountID,
		CrossAccount:           controlAccountID != "" && targetAccountID != "" && controlAccountID != targetAccountID,
		ObserverRoleConfigured: strings.TrimSpace(ctx.ObserverRoleARN) != "",
	}
}

type Clients struct {
	Config aws.Config
	ECS    *ecs.Client
	Logs   *cloudwatchlogs.Client
	ASG    *autoscaling.Client
	EC2    *ec2.Client
}

type ClientCacheDeps struct {
	LoadConfig    func(context.Context, string, string, string) (aws.Config, error)
	NewECSClient  func(aws.Config) *ecs.Client
	NewLogsClient func(aws.Config) *cloudwatchlogs.Client
	NewASGClient  func(aws.Config) *autoscaling.Client
	NewEC2Client  func(aws.Config) *ec2.Client
}

type clientCacheKey struct {
	region          string
	targetAccountID string
	observerRoleARN string
}

type clientCacheEntry struct {
	once    sync.Once
	clients Clients
	err     error
}

type ClientCache struct {
	deps    ClientCacheDeps
	entries sync.Map
}

func DefaultClientCacheDeps() ClientCacheDeps {
	return ClientCacheDeps{
		LoadConfig: LoadConfig,
		NewECSClient: func(cfg aws.Config) *ecs.Client {
			return ecs.NewFromConfig(cfg)
		},
		NewLogsClient: func(cfg aws.Config) *cloudwatchlogs.Client {
			return cloudwatchlogs.NewFromConfig(cfg)
		},
		NewASGClient: func(cfg aws.Config) *autoscaling.Client {
			return autoscaling.NewFromConfig(cfg)
		},
		NewEC2Client: func(cfg aws.Config) *ec2.Client {
			return ec2.NewFromConfig(cfg)
		},
	}
}

func NewClientCache(deps ClientCacheDeps) *ClientCache {
	return &ClientCache{deps: deps}
}

func (c *ClientCache) Clients(ctx context.Context, region, targetAccountID, observerRoleARN string) (Clients, error) {
	key := clientCacheKey{
		region:          strings.TrimSpace(region),
		targetAccountID: strings.TrimSpace(targetAccountID),
		observerRoleARN: strings.TrimSpace(observerRoleARN),
	}
	actual, _ := c.entries.LoadOrStore(key, &clientCacheEntry{})
	entry := actual.(*clientCacheEntry)
	entry.once.Do(func() {
		cfg, err := c.deps.LoadConfig(ctx, key.region, key.targetAccountID, key.observerRoleARN)
		if err != nil {
			entry.err = err
			return
		}
		entry.clients = Clients{
			Config: cfg,
			ECS:    c.deps.NewECSClient(cfg),
			Logs:   c.deps.NewLogsClient(cfg),
			ASG:    c.deps.NewASGClient(cfg),
			EC2:    c.deps.NewEC2Client(cfg),
		}
	})
	if entry.err != nil {
		c.entries.Delete(key)
		return Clients{}, entry.err
	}
	return entry.clients, nil
}

func LoadConfig(ctx context.Context, region, targetAccountID, observerRoleARN string) (aws.Config, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return aws.Config{}, err
	}
	callerAccountID := ""
	identity, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err == nil {
		callerAccountID = aws.ToString(identity.Account)
	}
	if targetAccountID != "" && callerAccountID != "" && targetAccountID != callerAccountID {
		if strings.TrimSpace(observerRoleARN) == "" {
			return aws.Config{}, NewUnavailableError(UnavailableReasonCrossAccountNotConfigured)
		}
		cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), observerRoleARN, func(options *stscreds.AssumeRoleOptions) {
			options.RoleSessionName = "tenet-resource-observer"
		}))
	}
	return cfg, nil
}
