package resourcelogs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/DevopsArtFactory/tenet/internal/resourceaws"
)

const (
	SourceCloudWatch = "cloudwatch"
	SourceEC2Console = "ec2_console"
	SourceCodeDeploy = "codedeploy"

	DefaultMaxEntries = 50

	ScopeTruncationStatusTruncated     = "truncated"
	ScopeTruncationStatusNotTruncated  = "not_truncated"
	ScopeTruncationStatusUnknown       = "unknown"
	ScopeTruncationStatusNotApplicable = "not_applicable"

	UnavailableReasonUnsupportedResourceType   = resourceaws.UnavailableReasonUnsupportedResourceType
	UnavailableReasonCrossAccountNotConfigured = resourceaws.UnavailableReasonCrossAccountNotConfigured
	UnavailableReasonNoLogSource               = resourceaws.UnavailableReasonNoLogSource
	UnavailableReasonNoRunningTasks            = resourceaws.UnavailableReasonNoRunningTasks
	UnavailableReasonNoInstances               = resourceaws.UnavailableReasonNoInstances
	UnavailableReasonResourceNotFound          = resourceaws.UnavailableReasonResourceNotFound
	UnavailableReasonAWSUnavailable            = resourceaws.UnavailableReasonAWSUnavailable

	EmptyReasonNoRecentEvents     = "no_recent_events"
	EmptyReasonNoFormattedEntries = "no_formatted_entries"
)

var ErrNotImplemented = errors.New("resource log service not implemented")

type containerNameContextKey struct{}
type instanceIDContextKey struct{}
type clusterARNContextKey struct{}

func ContextWithContainerName(ctx context.Context, containerName string) context.Context {
	containerName = strings.TrimSpace(containerName)
	if containerName == "" {
		return ctx
	}
	return context.WithValue(ctx, containerNameContextKey{}, containerName)
}

func ContainerNameFromContext(ctx context.Context) string {
	value, _ := ctx.Value(containerNameContextKey{}).(string)
	return strings.TrimSpace(value)
}

func ContextWithInstanceID(ctx context.Context, instanceID string) context.Context {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return ctx
	}
	return context.WithValue(ctx, instanceIDContextKey{}, instanceID)
}

func InstanceIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(instanceIDContextKey{}).(string)
	return strings.TrimSpace(value)
}

func ContextWithClusterARN(ctx context.Context, clusterARN string) context.Context {
	clusterARN = strings.TrimSpace(clusterARN)
	if clusterARN == "" {
		return ctx
	}
	return context.WithValue(ctx, clusterARNContextKey{}, clusterARN)
}

func ClusterARNFromContext(ctx context.Context) string {
	value, _ := ctx.Value(clusterARNContextKey{}).(string)
	return strings.TrimSpace(value)
}

type Request struct {
	DeploymentName      string
	ResourceType        string
	ResourceName        string
	ResourceDescription string
	BackingResourceType string
	BackingResourceName string
	ControlAccountID    string
	TargetAccountID     string
	Region              string
	ObserverRoleARN     string
	ContainerName       string
	InstanceID          string
	ClusterARN          string
	Limit               int
}

type Result struct {
	DeploymentName      string        `json:"deploymentName"`
	ResourceType        string        `json:"resourceType"`
	ResourceName        string        `json:"resourceName"`
	ResourceDescription string        `json:"resourceDescription,omitempty"`
	BackingResourceType string        `json:"backingResourceType,omitempty"`
	BackingResourceName string        `json:"backingResourceName,omitempty"`
	ContainerName       string        `json:"containerName,omitempty"`
	InstanceID          string        `json:"instanceId,omitempty"`
	ClusterARN          string        `json:"clusterArn,omitempty"`
	LookupContext       LookupContext `json:"lookupContext"`
	Available           bool          `json:"available"`
	Source              string        `json:"source"`
	Entries             []Entry       `json:"entries"`
	ResultScope         *ResultScope  `json:"resultScope,omitempty"`
	UnavailableReason   string        `json:"unavailableReason"`
	EmptyReason         string        `json:"emptyReason,omitempty"`
}

// ResultScope is additive operator-facing metadata for bounded recent-log reads.
// It describes the cap Tenet applied to this response without promising an
// expensive provider-wide total count.
type ResultScope struct {
	RequestedLimit       *int   `json:"requestedLimit,omitempty"`
	EffectiveLimit       int    `json:"effectiveLimit"`
	ReturnedEntryCount   int    `json:"returnedEntryCount"`
	NewestEntryTimestamp string `json:"newestEntryTimestamp,omitempty"`
	OldestEntryTimestamp string `json:"oldestEntryTimestamp,omitempty"`
	TruncationStatus     string `json:"truncationStatus"`
}

type LookupContext = resourceaws.LookupContext

type Entry struct {
	EntryID       string `json:"entryId,omitempty"`
	Timestamp     string `json:"timestamp"`
	Message       string `json:"message"`
	Stream        string `json:"stream,omitempty"`
	LogGroup      string `json:"logGroup,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
	InstanceID    string `json:"instanceId,omitempty"`
}

type Options struct {
	MaxEntries              int
	EnableAWS               bool
	ListECSLogEvents        func(context.Context, ECSRequest) ([]CloudWatchEvent, error)
	ListASGInstanceConsoles func(context.Context, ASGRequest) ([]InstanceConsole, error)
	ListCodeDeployEvents    func(context.Context, CodeDeployRequest) ([]CodeDeployEvent, error)
}

type Service struct {
	options  Options
	awsCache *resourceaws.ClientCache
}

func NewService(options Options) Service {
	return Service{
		options:  options,
		awsCache: newAWSClientCache(defaultAWSClientCacheDeps()),
	}
}

type awsClients = resourceaws.Clients
type awsClientCacheDeps = resourceaws.ClientCacheDeps

func defaultAWSClientCacheDeps() awsClientCacheDeps {
	return resourceaws.DefaultClientCacheDeps()
}

func newAWSClientCache(deps awsClientCacheDeps) *resourceaws.ClientCache {
	return resourceaws.NewClientCache(deps)
}

func (s Service) awsClients(ctx context.Context, region, targetAccountID, observerRoleARN string) (awsClients, error) {
	cache := s.awsCache
	if cache == nil {
		cache = newAWSClientCache(defaultAWSClientCacheDeps())
	}
	return cache.Clients(ctx, region, targetAccountID, observerRoleARN)
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
			ClusterARN:      clusterARN(req),
			Limit:           req.Limit,
		})
		if err != nil {
			if reason, ok := unavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		entries := formatCloudWatchEntries(events, s.effectiveLimit(req.Limit))
		return availableResult(req, SourceCloudWatch, entries, emptyReason(len(events), len(entries))), nil
	case "ecsTask":
		list := s.options.ListECSLogEvents
		if list == nil {
			if !s.options.EnableAWS {
				return Result{}, ErrNotImplemented
			}
			list = s.listECSTaskLogEventsFromAWS
		}
		events, err := list(ctx, ECSRequest{
			DeploymentName:  req.DeploymentName,
			ResourceName:    req.ResourceName,
			Region:          req.Region,
			TargetAccountID: req.TargetAccountID,
			ObserverRoleARN: req.ObserverRoleARN,
			ContainerName:   req.ContainerName,
			ClusterARN:      clusterARN(req),
			Limit:           req.Limit,
		})
		if err != nil {
			if reason, ok := unavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		entries := formatCloudWatchEntries(events, s.effectiveLimit(req.Limit), req.ContainerName)
		return availableResult(req, SourceCloudWatch, entries, emptyReason(len(events), len(entries))), nil
	case "candidateAutoScalingGroup", "serviceAutoScalingGroup", "userDataBootstrapSource":
		resourceName := req.ResourceName
		if req.ResourceType == "userDataBootstrapSource" {
			resourceName = strings.TrimSpace(req.BackingResourceName)
			if resourceName == "" {
				return unavailableResult(req, UnavailableReasonNoLogSource), nil
			}
		}
		list := s.options.ListASGInstanceConsoles
		if list == nil {
			if !s.options.EnableAWS {
				return Result{}, ErrNotImplemented
			}
			list = s.listASGInstanceConsolesFromAWS
		}
		consoles, err := list(ctx, ASGRequest{
			DeploymentName:  req.DeploymentName,
			ResourceName:    resourceName,
			Region:          req.Region,
			TargetAccountID: req.TargetAccountID,
			ObserverRoleARN: req.ObserverRoleARN,
			InstanceID:      instanceID(req),
			Limit:           req.Limit,
		})
		if err != nil {
			if reason, ok := unavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		consoles = selectInstanceConsoles(consoles, instanceID(req))
		if instanceID(req) != "" && len(consoles) == 0 {
			return unavailableResult(req, UnavailableReasonResourceNotFound), nil
		}
		entries := formatConsoleEntries(consoles, s.effectiveLimit(req.Limit))
		return availableResult(req, SourceEC2Console, entries, emptyReason(len(consoles), len(entries))), nil
	case "trafficShiftPlan", "productionListenerRule", "activeTargetGroup", "candidateTargetGroup":
		return unavailableResult(req, UnavailableReasonNoLogSource), nil
	case "codeDeployApplication", "codeDeployDeploymentGroup", "productionListener", "testListener", "blueTaskSetTargetGroup", "greenTaskSetTargetGroup":
		resourceName := req.ResourceName
		if req.ResourceType != "codeDeployApplication" && req.ResourceType != "codeDeployDeploymentGroup" {
			resourceName = strings.TrimSpace(req.BackingResourceName)
			if resourceName == "" {
				return unavailableResult(req, UnavailableReasonNoLogSource), nil
			}
		}
		list := s.options.ListCodeDeployEvents
		if list == nil {
			if !s.options.EnableAWS {
				return Result{}, ErrNotImplemented
			}
			list = s.listCodeDeployEventsFromAWS
		}
		events, err := list(ctx, CodeDeployRequest{
			DeploymentName:  req.DeploymentName,
			ResourceType:    req.ResourceType,
			ResourceName:    resourceName,
			Region:          req.Region,
			TargetAccountID: req.TargetAccountID,
			ObserverRoleARN: req.ObserverRoleARN,
			Limit:           req.Limit,
		})
		if err != nil {
			if reason, ok := unavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		entries := formatCodeDeployEntries(events, s.effectiveLimit(req.Limit))
		return availableResult(req, SourceCodeDeploy, entries, emptyReason(len(events), len(entries))), nil
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
	ContainerName   string
	ClusterARN      string
	Limit           int
}

type CloudWatchEvent struct {
	Timestamp     time.Time
	Message       string
	Stream        string
	LogGroup      string
	ContainerName string
}

type ASGRequest struct {
	DeploymentName  string
	ResourceName    string
	Region          string
	TargetAccountID string
	ObserverRoleARN string
	InstanceID      string
	Limit           int
}

type CodeDeployRequest struct {
	DeploymentName  string
	ResourceType    string
	ResourceName    string
	Region          string
	TargetAccountID string
	ObserverRoleARN string
	Limit           int
}

type CodeDeployEvent struct {
	Timestamp    time.Time
	Message      string
	DeploymentID string
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

func unavailableReasonForError(err error) (string, bool) {
	return resourceaws.UnavailableReasonForError(err)
}

func unavailableResult(req Request, reason string) Result {
	return Result{
		DeploymentName:      req.DeploymentName,
		ResourceType:        req.ResourceType,
		ResourceName:        req.ResourceName,
		ResourceDescription: resourceDescription(req),
		BackingResourceType: backingResourceType(req),
		BackingResourceName: backingResourceName(req),
		ContainerName:       containerName(req),
		InstanceID:          instanceID(req),
		ClusterARN:          clusterARN(req),
		LookupContext:       lookupContext(req),
		Available:           false,
		UnavailableReason:   reason,
	}
}

func availableResult(req Request, source string, entries []Entry, emptyReason string) Result {
	return Result{
		DeploymentName:      req.DeploymentName,
		ResourceType:        req.ResourceType,
		ResourceName:        req.ResourceName,
		ResourceDescription: resourceDescription(req),
		BackingResourceType: backingResourceType(req),
		BackingResourceName: backingResourceName(req),
		ContainerName:       containerName(req),
		InstanceID:          instanceID(req),
		ClusterARN:          clusterARN(req),
		LookupContext:       lookupContext(req),
		Available:           true,
		Source:              source,
		Entries:             entries,
		EmptyReason:         emptyReason,
	}
}

func emptyReason(rawCount, entryCount int) string {
	if entryCount > 0 {
		return ""
	}
	if rawCount > 0 {
		return EmptyReasonNoFormattedEntries
	}
	return EmptyReasonNoRecentEvents
}

func AssignEntryIDs(req Request, source string, entries []Entry) []Entry {
	if len(entries) == 0 {
		return entries
	}
	withIDs := make([]Entry, len(entries))
	copy(withIDs, entries)
	seen := make(map[string]int, len(entries))
	for i := range withIDs {
		base := entryIdentityBase(req, source, withIDs[i])
		seen[base]++
		withIDs[i].EntryID = stableEntryID(base, seen[base])
	}
	return withIDs
}

func entryIdentityBase(req Request, source string, entry Entry) string {
	parts := []string{
		"v1",
		strings.TrimSpace(req.DeploymentName),
		strings.TrimSpace(req.ResourceType),
		strings.TrimSpace(req.ResourceName),
		strings.TrimSpace(req.ResourceDescription),
		strings.TrimSpace(req.BackingResourceType),
		strings.TrimSpace(req.BackingResourceName),
		strings.TrimSpace(req.Region),
		strings.TrimSpace(req.ControlAccountID),
		strings.TrimSpace(req.TargetAccountID),
		strings.TrimSpace(req.ContainerName),
		strings.TrimSpace(req.InstanceID),
		strings.TrimSpace(req.ClusterARN),
		strings.TrimSpace(source),
		strings.TrimSpace(entry.Timestamp),
		strings.TrimSpace(entry.Stream),
		strings.TrimSpace(entry.InstanceID),
		strings.TrimSpace(entry.ContainerName),
		strings.TrimSpace(entry.Message),
	}
	return strings.Join(parts, "\x00")
}

func stableEntryID(base string, duplicateOrdinal int) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", base, duplicateOrdinal)))
	return "rlog_" + hex.EncodeToString(hash[:12])
}

func resourceDescription(req Request) string {
	return strings.TrimSpace(req.ResourceDescription)
}

func backingResourceType(req Request) string {
	return strings.TrimSpace(req.BackingResourceType)
}

func backingResourceName(req Request) string {
	return strings.TrimSpace(req.BackingResourceName)
}

func containerName(req Request) string {
	return strings.TrimSpace(req.ContainerName)
}

func instanceID(req Request) string {
	return strings.TrimSpace(req.InstanceID)
}

func clusterARN(req Request) string {
	if req.ResourceType != "ecsService" && req.ResourceType != "ecsTask" {
		return ""
	}
	return strings.TrimSpace(req.ClusterARN)
}

func lookupContext(req Request) LookupContext {
	return resourceaws.Lookup(resourceaws.RequestContext{
		ControlAccountID: req.ControlAccountID,
		TargetAccountID:  req.TargetAccountID,
		Region:           req.Region,
		ObserverRoleARN:  req.ObserverRoleARN,
	})
}

func (s Service) maxEntries() int {
	if s.options.MaxEntries > 0 {
		return s.options.MaxEntries
	}
	return DefaultMaxEntries
}

func (s Service) effectiveLimit(requested int) int {
	if requested > 0 {
		return requested
	}
	return s.maxEntries()
}

func formatCloudWatchEntries(events []CloudWatchEvent, max int, fallbackContainerName ...string) []Entry {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})
	containerFallback := ""
	if len(fallbackContainerName) > 0 {
		containerFallback = strings.TrimSpace(fallbackContainerName[0])
	}
	entries := make([]Entry, 0, min(max, len(events)))
	for _, event := range events {
		message := strings.TrimSpace(event.Message)
		if message == "" {
			continue
		}
		containerName := strings.TrimSpace(event.ContainerName)
		if containerName == "" {
			containerName = containerFallback
		}
		entries = append(entries, Entry{Timestamp: event.Timestamp.UTC().Format(time.RFC3339), Message: message, Stream: strings.TrimSpace(event.Stream), LogGroup: strings.TrimSpace(event.LogGroup), ContainerName: containerName})
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
			entries = append(entries, Entry{Timestamp: console.Lines[i].Timestamp.UTC().Format(time.RFC3339), Message: message, InstanceID: strings.TrimSpace(console.InstanceID)})
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

func selectInstanceConsoles(consoles []InstanceConsole, instanceID string) []InstanceConsole {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return consoles
	}
	selected := make([]InstanceConsole, 0, 1)
	for _, console := range consoles {
		if strings.TrimSpace(console.InstanceID) == instanceID {
			selected = append(selected, console)
		}
	}
	return selected
}

func formatCodeDeployEntries(events []CodeDeployEvent, max int) []Entry {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})
	entries := make([]Entry, 0, min(max, len(events)))
	for _, event := range events {
		message := strings.TrimSpace(event.Message)
		if message == "" {
			continue
		}
		entries = append(entries, Entry{
			Timestamp: event.Timestamp.UTC().Format(time.RFC3339),
			Message:   message,
			Stream:    strings.TrimSpace(event.DeploymentID),
		})
		if len(entries) == max {
			break
		}
	}
	return entries
}

func (s Service) listECSLogEventsFromAWS(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return nil, err
	}

	service, clusterARN, err := findECSService(ctx, clients.ECS, req.ResourceName, strings.TrimSpace(req.ClusterARN))
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonResourceNotFound)
	}
	tasks, err := findRecentServiceTasks(ctx, clients.ECS, aws.ToString(clusterARN), aws.ToString(service.ServiceName))
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonNoRunningTasks)
	}

	events, err := fetchRecentServiceTaskLogEvents(ctx, clients.ECS, clients.Logs, tasks, s.effectiveLimit(req.Limit))
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonNoLogSource)
	}
	return events, nil
}

func (s Service) listECSTaskLogEventsFromAWS(ctx context.Context, req ECSRequest) ([]CloudWatchEvent, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return nil, err
	}
	task, err := findTaskByARN(ctx, clients.ECS, req.ResourceName, strings.TrimSpace(req.ClusterARN))
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonResourceNotFound)
	}
	definitionARN := aws.ToString(task.TaskDefinitionArn)
	if definitionARN == "" {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonNoLogSource)
	}
	out, err := clients.ECS.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(definitionARN)})
	if err != nil {
		return nil, err
	}
	if out.TaskDefinition == nil {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonNoLogSource)
	}
	events, err := fetchTaskLogEventsForContainer(ctx, clients.Logs, *task, *out.TaskDefinition, s.effectiveLimit(req.Limit), req.ContainerName)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonNoLogSource)
	}
	return events, nil
}

func (s Service) listASGInstanceConsolesFromAWS(ctx context.Context, req ASGRequest) ([]InstanceConsole, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return nil, err
	}

	groupsOut, err := clients.ASG.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{AutoScalingGroupNames: []string{req.ResourceName}})
	if err != nil {
		return nil, err
	}
	if len(groupsOut.AutoScalingGroups) == 0 {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonResourceNotFound)
	}
	group := groupsOut.AutoScalingGroups[0]
	if len(group.Instances) == 0 {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonNoInstances)
	}
	instanceID := strings.TrimSpace(req.InstanceID)
	if instanceID != "" {
		if !asgHasInstance(group.Instances, instanceID) {
			return nil, resourceaws.NewUnavailableError(UnavailableReasonResourceNotFound)
		}
	} else {
		var err error
		instanceID, err = newestInstanceID(ctx, clients.EC2, group.Instances)
		if err != nil {
			return nil, err
		}
	}
	if instanceID == "" {
		return nil, resourceaws.NewUnavailableError(UnavailableReasonNoInstances)
	}
	consoleOut, err := clients.EC2.GetConsoleOutput(ctx, &ec2.GetConsoleOutputInput{InstanceId: aws.String(instanceID), Latest: aws.Bool(true)})
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

func (s Service) listCodeDeployEventsFromAWS(ctx context.Context, req CodeDeployRequest) ([]CodeDeployEvent, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return nil, err
	}
	applicationName := codeDeployApplicationName(req.DeploymentName)
	deploymentGroupName := strings.TrimSpace(req.ResourceName)
	if req.ResourceType == "codeDeployApplication" {
		applicationName = strings.TrimSpace(req.ResourceName)
		deploymentGroupName = ""
	}
	infos, err := listCodeDeployDeploymentInfos(ctx, clients.Config, req.Region, applicationName, deploymentGroupName, s.effectiveLimit(req.Limit))
	if err != nil {
		return nil, err
	}
	events := make([]CodeDeployEvent, 0, len(infos))
	for _, info := range infos {
		events = append(events, CodeDeployEvent{
			Timestamp:    codeDeployDeploymentTimestamp(info),
			Message:      codeDeployDeploymentMessage(info),
			DeploymentID: strings.TrimSpace(info.DeploymentID),
		})
	}
	return events, nil
}

type ecsServiceClient interface {
	ListClusters(context.Context, *ecs.ListClustersInput, ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
}

func describeECSServiceInCluster(ctx context.Context, client ecsServiceClient, clusterARN, serviceName string) (*ecstypes.Service, error) {
	out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(clusterARN), Services: []string{serviceName}})
	if err != nil {
		return nil, err
	}
	for _, service := range out.Services {
		if aws.ToString(service.ServiceName) == serviceName {
			return &service, nil
		}
	}
	return nil, nil
}

func findECSService(ctx context.Context, client ecsServiceClient, serviceName, selectedClusterARN string) (*ecstypes.Service, *string, error) {
	selectedClusterARN = strings.TrimSpace(selectedClusterARN)
	if selectedClusterARN != "" {
		service, err := describeECSServiceInCluster(ctx, client, selectedClusterARN, serviceName)
		if err != nil {
			return nil, nil, err
		}
		if service == nil {
			return nil, nil, nil
		}
		return service, aws.String(selectedClusterARN), nil
	}

	clusters := ecs.NewListClustersPaginator(client, &ecs.ListClustersInput{})
	for clusters.HasMorePages() {
		page, err := clusters.NextPage(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, clusterARN := range page.ClusterArns {
			service, err := describeECSServiceInCluster(ctx, client, clusterARN, serviceName)
			if err != nil {
				return nil, nil, err
			}
			if service != nil {
				return service, aws.String(clusterARN), nil
			}
		}
	}
	return nil, nil, nil
}

type ecsTaskClient interface {
	ListTasks(context.Context, *ecs.ListTasksInput, ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
}

type ecsTaskLookupClient interface {
	ListClusters(context.Context, *ecs.ListClustersInput, ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
}

type ecsTaskDefinitionClient interface {
	DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
}

type cloudWatchLogEventsClient interface {
	GetLogEvents(context.Context, *cloudwatchlogs.GetLogEventsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetLogEventsOutput, error)
}

func findRecentServiceTasks(ctx context.Context, client ecsTaskClient, clusterARN, serviceName string) ([]ecstypes.Task, error) {
	seen := map[string]struct{}{}
	tasks := make([]ecstypes.Task, 0)
	for _, desiredStatus := range []ecstypes.DesiredStatus{ecstypes.DesiredStatusRunning, ecstypes.DesiredStatusStopped} {
		statusTasks, err := findRecentServiceTasksByStatus(ctx, client, clusterARN, serviceName, desiredStatus)
		if err != nil {
			return nil, err
		}
		for _, task := range statusTasks {
			arn := aws.ToString(task.TaskArn)
			if arn == "" {
				continue
			}
			if _, ok := seen[arn]; ok {
				continue
			}
			seen[arn] = struct{}{}
			tasks = append(tasks, task)
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return taskRecency(tasks[i]).After(taskRecency(tasks[j]))
	})
	return tasks, nil
}

func findRecentServiceTasksByStatus(ctx context.Context, client ecsTaskClient, clusterARN, serviceName string, desiredStatus ecstypes.DesiredStatus) ([]ecstypes.Task, error) {
	listOut, err := client.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster:       aws.String(clusterARN),
		ServiceName:   aws.String(serviceName),
		DesiredStatus: desiredStatus,
		MaxResults:    aws.Int32(10),
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
		return taskRecency(tasks[i]).After(taskRecency(tasks[j]))
	})
	return tasks, nil
}

func taskRecency(task ecstypes.Task) time.Time {
	for _, candidate := range []time.Time{
		aws.ToTime(task.StoppedAt),
		aws.ToTime(task.StartedAt),
		aws.ToTime(task.PullStartedAt),
		aws.ToTime(task.CreatedAt),
	} {
		if !candidate.IsZero() {
			return candidate
		}
	}
	return time.Time{}
}

func findTaskByARN(ctx context.Context, client ecsTaskLookupClient, taskARN, selectedClusterARN string) (*ecstypes.Task, error) {
	taskARN = strings.TrimSpace(taskARN)
	if taskARN == "" {
		return nil, nil
	}
	selectedClusterARN = strings.TrimSpace(selectedClusterARN)
	clusters := candidateClustersFromTaskARN(taskARN)
	if selectedClusterARN != "" {
		clusters = []string{selectedClusterARN}
	}
	if len(clusters) == 0 {
		pages := ecs.NewListClustersPaginator(client, &ecs.ListClustersInput{})
		for pages.HasMorePages() {
			page, err := pages.NextPage(ctx)
			if err != nil {
				return nil, err
			}
			clusters = append(clusters, page.ClusterArns...)
		}
	}
	for _, cluster := range clusters {
		out, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: aws.String(cluster), Tasks: []string{taskARN}})
		if err != nil {
			return nil, err
		}
		for _, task := range out.Tasks {
			if aws.ToString(task.TaskArn) == taskARN || taskIDFromARN(aws.ToString(task.TaskArn)) == taskIDFromARN(taskARN) {
				copy := task
				return &copy, nil
			}
		}
	}
	return nil, nil
}

func candidateClustersFromTaskARN(taskARN string) []string {
	idx := strings.LastIndex(taskARN, ":task/")
	if idx < 0 {
		return nil
	}
	rest := taskARN[idx+len(":task/"):]
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return nil
	}
	return []string{parts[0]}
}

func fetchRecentServiceTaskLogEvents(ctx context.Context, ecsClient ecsTaskDefinitionClient, logsClient cloudWatchLogEventsClient, tasks []ecstypes.Task, limit int) ([]CloudWatchEvent, error) {
	taskDefinitions := map[string]ecstypes.TaskDefinition{}
	events := make([]CloudWatchEvent, 0)
	for _, task := range tasks {
		taskDefinitionARN := aws.ToString(task.TaskDefinitionArn)
		if taskDefinitionARN == "" {
			continue
		}
		taskDefinition, ok := taskDefinitions[taskDefinitionARN]
		if !ok {
			out, err := ecsClient.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(taskDefinitionARN)})
			if err != nil {
				return nil, err
			}
			if out.TaskDefinition == nil {
				continue
			}
			taskDefinition = *out.TaskDefinition
			taskDefinitions[taskDefinitionARN] = taskDefinition
		}
		taskEvents, err := fetchTaskLogEvents(ctx, logsClient, task, taskDefinition, limit)
		if err != nil {
			return nil, err
		}
		events = append(events, taskEvents...)
	}
	return events, nil
}

func fetchTaskLogEvents(ctx context.Context, client cloudWatchLogEventsClient, task ecstypes.Task, definition ecstypes.TaskDefinition, limit int) ([]CloudWatchEvent, error) {
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
				Timestamp:     time.UnixMilli(aws.ToInt64(event.Timestamp)).UTC(),
				Message:       aws.ToString(event.Message),
				Stream:        streamName,
				LogGroup:      groupName,
				ContainerName: aws.ToString(container.Name),
			})
		}
	}
	return events, nil
}

func fetchTaskLogEventsForContainer(ctx context.Context, client cloudWatchLogEventsClient, task ecstypes.Task, definition ecstypes.TaskDefinition, limit int, containerName string) ([]CloudWatchEvent, error) {
	containerName = strings.TrimSpace(containerName)
	containerByName := make(map[string]ecstypes.ContainerDefinition, len(definition.ContainerDefinitions))
	for _, container := range definition.ContainerDefinitions {
		containerByName[aws.ToString(container.Name)] = container
	}
	var events []CloudWatchEvent
	taskID := taskIDFromARN(aws.ToString(task.TaskArn))
	for _, container := range task.Containers {
		name := aws.ToString(container.Name)
		if containerName != "" && name != containerName {
			continue
		}
		definition, ok := containerByName[name]
		if !ok {
			continue
		}
		groupName, streamName := awsLogsLocation(definition, taskID)
		if groupName == "" || streamName == "" {
			continue
		}
		out, err := client.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{LogGroupName: aws.String(groupName), LogStreamName: aws.String(streamName), Limit: aws.Int32(int32(limit)), StartFromHead: aws.Bool(false)})
		if err != nil {
			var resourceMissing *cwtypes.ResourceNotFoundException
			if errors.As(err, &resourceMissing) {
				continue
			}
			return nil, err
		}
		for _, event := range out.Events {
			events = append(events, CloudWatchEvent{
				Timestamp:     time.UnixMilli(aws.ToInt64(event.Timestamp)).UTC(),
				Message:       aws.ToString(event.Message),
				Stream:        streamName,
				LogGroup:      groupName,
				ContainerName: name,
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

func asgHasInstance(instances []asgtypes.Instance, instanceID string) bool {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return false
	}
	for _, instance := range instances {
		if aws.ToString(instance.InstanceId) == instanceID {
			return true
		}
	}
	return false
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

func codeDeployApplicationName(deploymentName string) string {
	return strings.TrimSpace(deploymentName) + "-codedeploy-app"
}

func listCodeDeployDeploymentInfos(ctx context.Context, cfg aws.Config, region, applicationName, deploymentGroupName string, limit int) ([]codeDeployDeploymentInfoPayload, error) {
	ids, err := listCodeDeployDeploymentIDs(ctx, cfg, region, applicationName, deploymentGroupName, limit)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []codeDeployDeploymentInfoPayload{}, nil
	}
	infos := make([]codeDeployDeploymentInfoPayload, 0, len(ids))
	for start := 0; start < len(ids); start += 25 {
		end := min(start+25, len(ids))
		chunk, err := batchGetCodeDeployDeployments(ctx, cfg, region, ids[start:end])
		if err != nil {
			return nil, err
		}
		infos = append(infos, chunk...)
	}
	return infos, nil
}

func listCodeDeployDeploymentIDs(ctx context.Context, cfg aws.Config, region, applicationName, deploymentGroupName string, limit int) ([]string, error) {
	ids := make([]string, 0, limit)
	nextToken := ""
	for len(ids) < limit {
		var response codeDeployListDeploymentsOutput
		payload := codeDeployListDeploymentsPayload(applicationName, deploymentGroupName, nextToken)
		if err := doSignedCodeDeployJSONRequest(ctx, cfg, region, "CodeDeploy_20141006.ListDeployments", payload, &response); err != nil {
			return nil, err
		}
		for _, id := range response.Deployments {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			ids = append(ids, id)
			if len(ids) == limit {
				break
			}
		}
		nextToken = strings.TrimSpace(response.NextToken)
		if nextToken == "" {
			break
		}
	}
	return ids, nil
}

func batchGetCodeDeployDeployments(ctx context.Context, cfg aws.Config, region string, deploymentIDs []string) ([]codeDeployDeploymentInfoPayload, error) {
	var response codeDeployBatchGetDeploymentsOutput
	if err := doSignedCodeDeployJSONRequest(ctx, cfg, region, "CodeDeploy_20141006.BatchGetDeployments", map[string]any{"deploymentIds": deploymentIDs}, &response); err != nil {
		return nil, err
	}
	return response.DeploymentsInfo, nil
}

func codeDeployListDeploymentsPayload(applicationName, deploymentGroupName, nextToken string) map[string]any {
	payload := map[string]any{
		"applicationName": strings.TrimSpace(applicationName),
	}
	if deploymentGroupName := strings.TrimSpace(deploymentGroupName); deploymentGroupName != "" {
		payload["deploymentGroupName"] = deploymentGroupName
	}
	if nextToken != "" {
		payload["nextToken"] = nextToken
	}
	return payload
}

func doSignedCodeDeployJSONRequest(ctx context.Context, cfg aws.Config, region, target string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://codedeploy."+strings.TrimSpace(region)+".amazonaws.com/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-amz-json-1.1")
	request.Header.Set("X-Amz-Target", target)
	responseBody, responseHeaders, err := doSignedAWSRequest(ctx, cfg, "codedeploy", region, request, body)
	if err != nil {
		code := parseAWSJSONError(responseHeaders, responseBody)
		switch code {
		case "ApplicationDoesNotExistException", "DeploymentGroupDoesNotExistException", "DeploymentDoesNotExistException":
			return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
		default:
			return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)
		}
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return err
	}
	return nil
}

func doSignedAWSRequest(ctx context.Context, cfg aws.Config, service string, region string, request *http.Request, body []byte) ([]byte, http.Header, error) {
	credentials, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, nil, err
	}
	hash := sha256.Sum256(body)
	if err := v4.NewSigner().SignHTTP(ctx, credentials, request, hex.EncodeToString(hash[:]), service, region, time.Now().UTC()); err != nil {
		return nil, nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, err
	}
	if response.StatusCode >= http.StatusBadRequest {
		return responseBody, response.Header, fmt.Errorf("aws request failed with status %d", response.StatusCode)
	}
	return responseBody, response.Header, nil
}

func parseAWSJSONError(headers http.Header, body []byte) string {
	type payload struct {
		Type string `json:"__type"`
		Code string `json:"code"`
	}
	var errPayload payload
	_ = json.Unmarshal(body, &errPayload)
	code := strings.TrimSpace(headers.Get("X-Amzn-Errortype"))
	if code == "" {
		code = strings.TrimSpace(errPayload.Type)
	}
	if code == "" {
		code = strings.TrimSpace(errPayload.Code)
	}
	if index := strings.LastIndex(code, "#"); index >= 0 {
		code = code[index+1:]
	}
	if index := strings.Index(code, ":"); index >= 0 {
		code = code[:index]
	}
	return strings.TrimSpace(code)
}

func codeDeployDeploymentTimestamp(info codeDeployDeploymentInfoPayload) time.Time {
	if !info.CompleteTime.IsZero() {
		return info.CompleteTime.UTC()
	}
	if !info.CreateTime.IsZero() {
		return info.CreateTime.UTC()
	}
	return time.Time{}
}

func codeDeployDeploymentMessage(info codeDeployDeploymentInfoPayload) string {
	parts := make([]string, 0, 4)
	if status := strings.TrimSpace(info.Status); status != "" {
		parts = append(parts, "Status: "+status)
	}
	if description := strings.TrimSpace(info.Description); description != "" {
		parts = append(parts, description)
	} else if len(info.DeploymentStatusMessages) > 0 {
		message := strings.TrimSpace(info.DeploymentStatusMessages[0])
		if message != "" {
			parts = append(parts, message)
		}
	}
	if message := strings.TrimSpace(info.ErrorInformation.Message); message != "" {
		parts = append(parts, "Error: "+message)
	}
	if len(parts) == 0 {
		if id := strings.TrimSpace(info.DeploymentID); id != "" {
			return "CodeDeploy deployment " + id
		}
		return "CodeDeploy deployment"
	}
	return strings.Join(parts, " — ")
}

type codeDeployListDeploymentsOutput struct {
	Deployments []string `json:"deployments"`
	NextToken   string   `json:"nextToken"`
}

type codeDeployBatchGetDeploymentsOutput struct {
	DeploymentsInfo []codeDeployDeploymentInfoPayload `json:"deploymentsInfo"`
}

type codeDeployDeploymentInfoPayload struct {
	DeploymentID             string                           `json:"deploymentId"`
	CreateTime               codeDeployUnixTime               `json:"createTime"`
	CompleteTime             codeDeployUnixTime               `json:"completeTime"`
	Status                   string                           `json:"status"`
	Description              string                           `json:"description"`
	DeploymentStatusMessages []string                         `json:"deploymentStatusMessages"`
	ErrorInformation         codeDeployDeploymentErrorPayload `json:"errorInformation"`
}

type codeDeployDeploymentErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type codeDeployUnixTime struct{ time.Time }

func (t *codeDeployUnixTime) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		t.Time = time.Time{}
		return nil
	}
	var seconds float64
	if err := json.Unmarshal(data, &seconds); err != nil {
		return err
	}
	whole, frac := math.Modf(seconds)
	t.Time = time.Unix(int64(whole), int64(frac*float64(time.Second))).UTC()
	return nil
}
