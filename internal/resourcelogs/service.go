package resourcelogs

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

const (
	SourceCloudWatch = "cloudwatch"
	SourceEC2Console = "ec2_console"

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

var ErrNotImplemented = errors.New("resource log service not implemented")

type Request struct {
	DeploymentName   string
	ResourceType     string
	ResourceName     string
	ControlAccountID string
	TargetAccountID  string
	Region           string
	ObserverRoleARN  string
}

type Result struct {
	DeploymentName    string        `json:"deploymentName"`
	ResourceType      string        `json:"resourceType"`
	ResourceName      string        `json:"resourceName"`
	LookupContext     LookupContext `json:"lookupContext"`
	Available         bool          `json:"available"`
	Source            string        `json:"source"`
	Entries           []Entry       `json:"entries"`
	UnavailableReason string        `json:"unavailableReason"`
}

type LookupContext struct {
	Region                 string `json:"region"`
	ControlAccountID       string `json:"controlAccountId"`
	TargetAccountID        string `json:"targetAccountId"`
	CrossAccount           bool   `json:"crossAccount"`
	ObserverRoleConfigured bool   `json:"observerRoleConfigured"`
}

type Entry struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Stream    string `json:"stream,omitempty"`
}

type Options struct {
	MaxEntries              int
	EnableAWS               bool
	ListECSLogEvents        func(context.Context, ECSRequest) ([]CloudWatchEvent, error)
	ListASGInstanceConsoles func(context.Context, ASGRequest) ([]InstanceConsole, error)
}

type Service struct {
	options  Options
	awsCache *awsClientCache
}

func NewService(options Options) Service {
	return Service{
		options:  options,
		awsCache: newAWSClientCache(defaultAWSClientCacheDeps()),
	}
}

type awsClientCacheKey struct {
	region          string
	targetAccountID string
	observerRoleARN string
}

type awsClients struct {
	ecs  *ecs.Client
	logs *cloudwatchlogs.Client
	asg  *autoscaling.Client
	ec2  *ec2.Client
}

type awsClientCacheEntry struct {
	once    sync.Once
	clients awsClients
	err     error
}

type awsClientCacheDeps struct {
	loadConfig    func(context.Context, string, string, string) (aws.Config, error)
	newECSClient  func(aws.Config) *ecs.Client
	newLogsClient func(aws.Config) *cloudwatchlogs.Client
	newASGClient  func(aws.Config) *autoscaling.Client
	newEC2Client  func(aws.Config) *ec2.Client
}

type awsClientCache struct {
	deps    awsClientCacheDeps
	entries sync.Map
}

func defaultAWSClientCacheDeps() awsClientCacheDeps {
	return awsClientCacheDeps{
		loadConfig: loadAWSConfig,
		newECSClient: func(cfg aws.Config) *ecs.Client {
			return ecs.NewFromConfig(cfg)
		},
		newLogsClient: func(cfg aws.Config) *cloudwatchlogs.Client {
			return cloudwatchlogs.NewFromConfig(cfg)
		},
		newASGClient: func(cfg aws.Config) *autoscaling.Client {
			return autoscaling.NewFromConfig(cfg)
		},
		newEC2Client: func(cfg aws.Config) *ec2.Client {
			return ec2.NewFromConfig(cfg)
		},
	}
}

func newAWSClientCache(deps awsClientCacheDeps) *awsClientCache {
	return &awsClientCache{deps: deps}
}

func (c *awsClientCache) clients(ctx context.Context, region, targetAccountID, observerRoleARN string) (awsClients, error) {
	key := awsClientCacheKey{
		region:          strings.TrimSpace(region),
		targetAccountID: strings.TrimSpace(targetAccountID),
		observerRoleARN: strings.TrimSpace(observerRoleARN),
	}
	actual, _ := c.entries.LoadOrStore(key, &awsClientCacheEntry{})
	entry := actual.(*awsClientCacheEntry)
	entry.once.Do(func() {
		cfg, err := c.deps.loadConfig(ctx, key.region, key.targetAccountID, key.observerRoleARN)
		if err != nil {
			entry.err = err
			return
		}
		entry.clients = awsClients{
			ecs:  c.deps.newECSClient(cfg),
			logs: c.deps.newLogsClient(cfg),
			asg:  c.deps.newASGClient(cfg),
			ec2:  c.deps.newEC2Client(cfg),
		}
	})
	if entry.err != nil {
		c.entries.Delete(key)
		return awsClients{}, entry.err
	}
	return entry.clients, nil
}

func (s Service) awsClients(ctx context.Context, region, targetAccountID, observerRoleARN string) (awsClients, error) {
	cache := s.awsCache
	if cache == nil {
		cache = newAWSClientCache(defaultAWSClientCacheDeps())
	}
	return cache.clients(ctx, region, targetAccountID, observerRoleARN)
}

func (s Service) Recent(ctx context.Context, req Request) (Result, error) {
	if req.ControlAccountID != "" && req.TargetAccountID != "" && req.ControlAccountID != req.TargetAccountID && strings.TrimSpace(req.ObserverRoleARN) == "" {
		return unavailableResult(req, UnavailableReasonCrossAccountNotConfigured), nil
	}

	switch req.ResourceType {
	case "ecsService":
		list := s.options.ListECSLogEvents
		if list == nil {
			if !s.options.EnableAWS {
				return Result{}, ErrNotImplemented
			}
			list = s.listECSLogEventsFromAWS
		}
		events, err := list(ctx, ECSRequest{
			DeploymentName:  req.DeploymentName,
			ResourceName:    req.ResourceName,
			Region:          req.Region,
			TargetAccountID: req.TargetAccountID,
			ObserverRoleARN: req.ObserverRoleARN,
		})
		if err != nil {
			if reason, ok := unavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		entries := formatCloudWatchEntries(events, s.maxEntries())
		return availableResult(req, SourceCloudWatch, entries), nil
	case "candidateAutoScalingGroup", "serviceAutoScalingGroup":
		list := s.options.ListASGInstanceConsoles
		if list == nil {
			if !s.options.EnableAWS {
				return Result{}, ErrNotImplemented
			}
			list = s.listASGInstanceConsolesFromAWS
		}
		consoles, err := list(ctx, ASGRequest{
			DeploymentName:  req.DeploymentName,
			ResourceName:    req.ResourceName,
			Region:          req.Region,
			TargetAccountID: req.TargetAccountID,
			ObserverRoleARN: req.ObserverRoleARN,
		})
		if err != nil {
			if reason, ok := unavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		entries := formatConsoleEntries(consoles, s.maxEntries())
		return availableResult(req, SourceEC2Console, entries), nil
	default:
		return unavailableResult(req, UnavailableReasonUnsupportedResourceType), nil
	}
}

type ECSRequest struct {
	DeploymentName  string
	ResourceName    string
	Region          string
	TargetAccountID string
	ObserverRoleARN string
}

type CloudWatchEvent struct {
	Timestamp time.Time
	Message   string
	Stream    string
}

type ASGRequest struct {
	DeploymentName  string
	ResourceName    string
	Region          string
	TargetAccountID string
	ObserverRoleARN string
}

type InstanceConsole struct {
	InstanceID  string
	GeneratedAt time.Time
	Lines       []ConsoleLine
}

type ConsoleLine struct {
	Timestamp time.Time
	Message   string
}

type unavailableError struct{ reason string }

func (e unavailableError) Error() string { return e.reason }

func unavailableReasonForError(err error) (string, bool) {
	var unavailable unavailableError
	if errors.As(err, &unavailable) {
		return unavailable.reason, true
	}
	if isAWSUnavailableError(err) {
		return UnavailableReasonAWSUnavailable, true
	}
	return "", false
}

func isAWSUnavailableError(err error) bool {
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

func unavailableResult(req Request, reason string) Result {
	return Result{
		DeploymentName:    req.DeploymentName,
		ResourceType:      req.ResourceType,
		ResourceName:      req.ResourceName,
		LookupContext:     lookupContext(req),
		Available:         false,
		UnavailableReason: reason,
	}
}

func availableResult(req Request, source string, entries []Entry) Result {
	return Result{
		DeploymentName: req.DeploymentName,
		ResourceType:   req.ResourceType,
		ResourceName:   req.ResourceName,
		LookupContext:  lookupContext(req),
		Available:      true,
		Source:         source,
		Entries:        entries,
	}
}

func lookupContext(req Request) LookupContext {
	controlAccountID := strings.TrimSpace(req.ControlAccountID)
	targetAccountID := strings.TrimSpace(req.TargetAccountID)
	return LookupContext{
		Region:                 strings.TrimSpace(req.Region),
		ControlAccountID:       controlAccountID,
		TargetAccountID:        targetAccountID,
		CrossAccount:           controlAccountID != "" && targetAccountID != "" && controlAccountID != targetAccountID,
		ObserverRoleConfigured: strings.TrimSpace(req.ObserverRoleARN) != "",
	}
}

func (s Service) maxEntries() int {
	if s.options.MaxEntries > 0 {
		return s.options.MaxEntries
	}
	return 50
}

func formatCloudWatchEntries(events []CloudWatchEvent, max int) []Entry {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})
	entries := make([]Entry, 0, min(max, len(events)))
	for _, event := range events {
		message := strings.TrimSpace(event.Message)
		if message == "" {
			continue
		}
		entries = append(entries, Entry{Timestamp: event.Timestamp.UTC().Format(time.RFC3339), Message: message, Stream: strings.TrimSpace(event.Stream)})
		if len(entries) == max {
			break
		}
	}
	return entries
}

func formatConsoleEntries(consoles []InstanceConsole, max int) []Entry {
	sort.SliceStable(consoles, func(i, j int) bool {
		return consoles[i].GeneratedAt.After(consoles[j].GeneratedAt)
	})
	entries := make([]Entry, 0, min(max, len(consoles)))
	for _, console := range consoles {
		for i := len(console.Lines) - 1; i >= 0; i-- {
			message := strings.TrimSpace(console.Lines[i].Message)
			if message == "" {
				continue
			}
			entries = append(entries, Entry{Timestamp: console.Lines[i].Timestamp.UTC().Format(time.RFC3339), Message: message})
			if len(entries) == max {
				break
			}
		}
		if len(entries) == max {
			break
		}
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries
}

func (s Service) listECSLogEventsFromAWS(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return nil, err
	}

	service, clusterARN, err := findECSService(ctx, clients.ecs, req.ResourceName)
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, unavailableError{reason: UnavailableReasonResourceNotFound}
	}
	tasks, err := findRecentRunningTasks(ctx, clients.ecs, aws.ToString(clusterARN), aws.ToString(service.ServiceName))
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, unavailableError{reason: UnavailableReasonNoRunningTasks}
	}

	taskDefinitions := map[string]ecstypes.TaskDefinition{}
	events := make([]CloudWatchEvent, 0)
	for _, task := range tasks {
		taskDefinitionARN := aws.ToString(task.TaskDefinitionArn)
		if taskDefinitionARN == "" {
			continue
		}
		taskDefinition, ok := taskDefinitions[taskDefinitionARN]
		if !ok {
			out, err := clients.ecs.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(taskDefinitionARN)})
			if err != nil {
				return nil, err
			}
			if out.TaskDefinition == nil {
				continue
			}
			taskDefinition = *out.TaskDefinition
			taskDefinitions[taskDefinitionARN] = taskDefinition
		}
		taskEvents, err := fetchTaskLogEvents(ctx, clients.logs, task, taskDefinition, s.maxEntries())
		if err != nil {
			return nil, err
		}
		events = append(events, taskEvents...)
		if len(events) >= s.maxEntries() {
			break
		}
	}
	if len(events) == 0 {
		return nil, unavailableError{reason: UnavailableReasonNoLogSource}
	}
	return events, nil
}

func (s Service) listASGInstanceConsolesFromAWS(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return nil, err
	}

	groupsOut, err := clients.asg.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{AutoScalingGroupNames: []string{req.ResourceName}})
	if err != nil {
		return nil, err
	}
	if len(groupsOut.AutoScalingGroups) == 0 {
		return nil, unavailableError{reason: UnavailableReasonResourceNotFound}
	}
	group := groupsOut.AutoScalingGroups[0]
	if len(group.Instances) == 0 {
		return nil, unavailableError{reason: UnavailableReasonNoInstances}
	}
	instanceID, err := newestInstanceID(ctx, clients.ec2, group.Instances)
	if err != nil {
		return nil, err
	}
	if instanceID == "" {
		return nil, unavailableError{reason: UnavailableReasonNoInstances}
	}
	consoleOut, err := clients.ec2.GetConsoleOutput(ctx, &ec2.GetConsoleOutputInput{InstanceId: aws.String(instanceID), Latest: aws.Bool(true)})
	if err != nil {
		return nil, err
	}
	decoded, err := decodeConsoleOutput(consoleOut.Output)
	if err != nil {
		return nil, err
	}
	lines := parseConsoleLines(decoded, aws.ToTime(consoleOut.Timestamp))
	return []InstanceConsole{{InstanceID: instanceID, GeneratedAt: aws.ToTime(consoleOut.Timestamp), Lines: lines}}, nil
}

func loadAWSConfig(ctx context.Context, region, targetAccountID, observerRoleARN string) (aws.Config, error) {
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
			return aws.Config{}, unavailableError{reason: UnavailableReasonCrossAccountNotConfigured}
		}
		cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), observerRoleARN, func(options *stscreds.AssumeRoleOptions) {
			options.RoleSessionName = "tenet-resource-logs"
		}))
	}
	return cfg, nil
}

func findECSService(ctx context.Context, client *ecs.Client, serviceName string) (*ecstypes.Service, *string, error) {
	clusters := ecs.NewListClustersPaginator(client, &ecs.ListClustersInput{})
	for clusters.HasMorePages() {
		page, err := clusters.NextPage(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, clusterARN := range page.ClusterArns {
			out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(clusterARN), Services: []string{serviceName}})
			if err != nil {
				return nil, nil, err
			}
			for _, service := range out.Services {
				if aws.ToString(service.ServiceName) == serviceName {
					return &service, aws.String(clusterARN), nil
				}
			}
		}
	}
	return nil, nil, nil
}

func findRecentRunningTasks(ctx context.Context, client *ecs.Client, clusterARN, serviceName string) ([]ecstypes.Task, error) {
	listOut, err := client.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster:       aws.String(clusterARN),
		ServiceName:   aws.String(serviceName),
		DesiredStatus: ecstypes.DesiredStatusRunning,
	})
	if err != nil {
		return nil, err
	}
	if len(listOut.TaskArns) == 0 {
		return nil, nil
	}
	describeOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: aws.String(clusterARN), Tasks: listOut.TaskArns})
	if err != nil {
		return nil, err
	}
	tasks := append([]ecstypes.Task(nil), describeOut.Tasks...)
	sort.SliceStable(tasks, func(i, j int) bool {
		return aws.ToTime(tasks[i].StartedAt).After(aws.ToTime(tasks[j].StartedAt))
	})
	return tasks, nil
}

func fetchTaskLogEvents(ctx context.Context, client *cloudwatchlogs.Client, task ecstypes.Task, definition ecstypes.TaskDefinition, limit int) ([]CloudWatchEvent, error) {
	containerByName := make(map[string]ecstypes.ContainerDefinition, len(definition.ContainerDefinitions))
	for _, container := range definition.ContainerDefinitions {
		containerByName[aws.ToString(container.Name)] = container
	}
	var events []CloudWatchEvent
	taskID := taskIDFromARN(aws.ToString(task.TaskArn))
	for _, container := range task.Containers {
		definition, ok := containerByName[aws.ToString(container.Name)]
		if !ok {
			continue
		}
		groupName, streamName := awsLogsLocation(definition, taskID)
		if groupName == "" || streamName == "" {
			continue
		}
		out, err := client.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(groupName),
			LogStreamName: aws.String(streamName),
			Limit:         aws.Int32(int32(limit)),
			StartFromHead: aws.Bool(false),
		})
		if err != nil {
			var resourceMissing *cwtypes.ResourceNotFoundException
			if errors.As(err, &resourceMissing) {
				continue
			}
			return nil, err
		}
		for _, event := range out.Events {
			events = append(events, CloudWatchEvent{
				Timestamp: time.UnixMilli(aws.ToInt64(event.Timestamp)).UTC(),
				Message:   aws.ToString(event.Message),
				Stream:    streamName,
			})
		}
	}
	return events, nil
}

func awsLogsLocation(container ecstypes.ContainerDefinition, taskID string) (string, string) {
	if container.LogConfiguration == nil || container.LogConfiguration.LogDriver != ecstypes.LogDriverAwslogs {
		return "", ""
	}
	options := container.LogConfiguration.Options
	groupName := options["awslogs-group"]
	streamPrefix := options["awslogs-stream-prefix"]
	containerName := aws.ToString(container.Name)
	if groupName == "" || streamPrefix == "" || containerName == "" || taskID == "" {
		return "", ""
	}
	return groupName, fmt.Sprintf("%s/%s/%s", streamPrefix, containerName, taskID)
}

func taskIDFromARN(arn string) string {
	if arn == "" {
		return ""
	}
	parts := strings.Split(arn, "/")
	return parts[len(parts)-1]
}

func newestInstanceID(ctx context.Context, client *ec2.Client, instances []asgtypes.Instance) (string, error) {
	ids := make([]string, 0, len(instances))
	for _, instance := range instances {
		if id := aws.ToString(instance.InstanceId); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return "", nil
	}
	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: ids})
	if err != nil {
		return "", err
	}
	var newest *ec2types.Instance
	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			if newest == nil || aws.ToTime(instance.LaunchTime).After(aws.ToTime(newest.LaunchTime)) {
				copy := instance
				newest = &copy
			}
		}
	}
	if newest == nil {
		return "", nil
	}
	return aws.ToString(newest.InstanceId), nil
}

func decodeConsoleOutput(encoded *string) (string, error) {
	if encoded == nil || aws.ToString(encoded) == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(aws.ToString(encoded))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func parseConsoleLines(output string, fallback time.Time) []ConsoleLine {
	if output == "" {
		return nil
	}
	rawLines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	lines := make([]ConsoleLine, 0, len(rawLines))
	for _, raw := range rawLines {
		message := strings.TrimSpace(raw)
		if message == "" {
			continue
		}
		lines = append(lines, ConsoleLine{Timestamp: fallback.UTC(), Message: message})
	}
	return lines
}
