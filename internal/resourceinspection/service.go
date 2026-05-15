package resourceinspection

import (
	"container/list"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/DevopsArtFactory/tenet/internal/resourceaws"
)

const (
	SourceECSService                     = "ecs_service"
	SourceAutoScalingGroup               = "auto_scaling_group"
	SourceTrafficRoutingAWS              = "traffic_routing_aws"
	SourceRolloutSummary                 = "rollout_execution_summary"
	ecsServiceHintCacheCap               = 256
	trafficRoutingCacheCap               = 128
	maxECSServiceTaskSummaries           = 10
	maxAutoScalingGroupInstanceSummaries = maxECSServiceTaskSummaries
	ecsServiceTaskListPageLimit          = 10
)

var launchTemplateBootstrapSourcePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^\s*(?:export\s+)?(?:TENET_)?(?:USERDATA|BOOTSTRAP)_SOURCE\s*=\s*['"]?([^'"#\s]+)['"]?`),
	regexp.MustCompile(`(?im)^\s*(?:#\s*)?(?:tenet[-_\s]*)?(?:userdata|bootstrap)[-_\s]*source\s*[:=]\s*['"]?([^'"#\s]+)['"]?`),
	regexp.MustCompile(`(?im)(?:--userdata-source|--bootstrap-source)\s+['"]?([^'"\s]+)`),
}

var ErrNotImplemented = errors.New("resource inspection service not implemented")

type LookupContext = resourceaws.LookupContext

type instanceIDContextKey struct{}
type clusterARNContextKey struct{}

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
	DeploymentName           string
	ResourceType             string
	ResourceName             string
	ResourceDescription      string
	TaskSummaryLimit         int
	AutoScalingInstanceLimit int
	InstanceID               string
	ClusterARN               string
	BackingResourceType      string
	BackingResourceName      string
	ControlAccountID         string
	TargetAccountID          string
	Region                   string
	ObserverRoleARN          string
}

type Result struct {
	DeploymentName          string                             `json:"deploymentName"`
	ResourceType            string                             `json:"resourceType"`
	ResourceName            string                             `json:"resourceName"`
	ResourceDescription     string                             `json:"resourceDescription,omitempty"`
	InstanceID              string                             `json:"instanceId,omitempty"`
	ClusterARN              string                             `json:"clusterArn,omitempty"`
	BackingResourceType     string                             `json:"backingResourceType,omitempty"`
	BackingResourceName     string                             `json:"backingResourceName,omitempty"`
	LookupContext           LookupContext                      `json:"lookupContext"`
	Available               bool                               `json:"available"`
	Source                  string                             `json:"source"`
	ECSService              *ECSServiceInspection              `json:"ecsService,omitempty"`
	ECSTask                 *ECSTaskInspection                 `json:"ecsTask,omitempty"`
	AutoScalingGroup        *AutoScalingInspection             `json:"autoScalingGroup,omitempty"`
	UserDataBootstrapSource *UserDataBootstrapSourceInspection `json:"userDataBootstrapSource,omitempty"`
	TrafficRoutingResource  *TrafficRoutingInspection          `json:"trafficRoutingResource,omitempty"`
	ResultScope             *ResultScope                       `json:"resultScope,omitempty"`
	UnavailableReason       string                             `json:"unavailableReason"`
}

// ResultScope is additive operator-facing metadata for bounded live-inspection
// reads. It mirrors the nested ECS tasksScope / ASG instancesScope window in a
// stable top-level place so scripts and UI consumers do not need to understand
// each resource-specific body before explaining the applied sample cap.
type ResultScope struct {
	SampleType        string `json:"sampleType"`
	RequestedLimit    *int   `json:"requestedLimit,omitempty"`
	EffectiveLimit    int    `json:"effectiveLimit"`
	ReturnedItemCount int    `json:"returnedItemCount"`
	ReturnedCount     int    `json:"returnedCount"`
	TruncationStatus  string `json:"truncationStatus"`
	Truncated         bool   `json:"truncated"`
}

type UserDataBootstrapSourceInspection struct {
	DesiredBootstrapSource  string                            `json:"desiredBootstrapSource"`
	ObservedBootstrapSource string                            `json:"observedBootstrapSource,omitempty"`
	BackingResourceType     string                            `json:"backingResourceType,omitempty"`
	BackingResourceName     string                            `json:"backingResourceName,omitempty"`
	LaunchTemplate          *AutoScalingLaunchTemplateSummary `json:"launchTemplate,omitempty"`
	SelectedInstance        *AutoScalingGroupInstanceSummary  `json:"selectedInstance,omitempty"`
}

type TrafficRoutingInspection struct {
	Kind                      string                               `json:"kind"`
	DisplayName               string                               `json:"displayName"`
	InspectionMode            string                               `json:"inspectionMode"`
	Summary                   string                               `json:"summary"`
	CodeDeployApplication     *CodeDeployApplicationInspection     `json:"codeDeployApplication,omitempty"`
	CodeDeployDeploymentGroup *CodeDeployDeploymentGroupInspection `json:"codeDeployDeploymentGroup,omitempty"`
	Listener                  *ListenerInspection                  `json:"listener,omitempty"`
	ListenerRule              *ListenerRuleInspection              `json:"listenerRule,omitempty"`
	TargetGroup               *TargetGroupInspection               `json:"targetGroup,omitempty"`
}

type TargetGroupInspection struct {
	TargetGroupName            string                   `json:"targetGroupName"`
	TargetGroupARN             string                   `json:"targetGroupArn"`
	Protocol                   string                   `json:"protocol,omitempty"`
	Port                       int32                    `json:"port,omitempty"`
	TargetType                 string                   `json:"targetType,omitempty"`
	LoadBalancerARNs           []string                 `json:"loadBalancerArns,omitempty"`
	HealthCheckProtocol        string                   `json:"healthCheckProtocol,omitempty"`
	HealthCheckPort            string                   `json:"healthCheckPort,omitempty"`
	HealthCheckPath            string                   `json:"healthCheckPath,omitempty"`
	HealthCheckIntervalSeconds int32                    `json:"healthCheckIntervalSeconds,omitempty"`
	HealthCheckTimeoutSeconds  int32                    `json:"healthCheckTimeoutSeconds,omitempty"`
	HealthyThresholdCount      int32                    `json:"healthyThresholdCount,omitempty"`
	UnhealthyThresholdCount    int32                    `json:"unhealthyThresholdCount,omitempty"`
	TargetHealthCounts         map[string]int           `json:"targetHealthCounts,omitempty"`
	Targets                    []TargetHealthInspection `json:"targets,omitempty"`
}

type TargetHealthInspection struct {
	TargetID    string `json:"targetId"`
	Port        int32  `json:"port,omitempty"`
	State       string `json:"state,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Description string `json:"description,omitempty"`
}

type CodeDeployApplicationInspection struct {
	ApplicationName  string   `json:"applicationName"`
	ApplicationID    string   `json:"applicationId,omitempty"`
	ComputePlatform  string   `json:"computePlatform,omitempty"`
	LinkedToGitHub   bool     `json:"linkedToGitHub"`
	DeploymentGroups []string `json:"deploymentGroups,omitempty"`
}

type CodeDeployDeploymentGroupInspection struct {
	ApplicationName       string                        `json:"applicationName"`
	DeploymentGroupName   string                        `json:"deploymentGroupName"`
	DeploymentConfigName  string                        `json:"deploymentConfigName,omitempty"`
	ServiceRoleARN        string                        `json:"serviceRoleArn,omitempty"`
	DeploymentType        string                        `json:"deploymentType,omitempty"`
	DeploymentOption      string                        `json:"deploymentOption,omitempty"`
	ECSServices           []CodeDeployECSServiceBinding `json:"ecsServices,omitempty"`
	TargetGroups          []string                      `json:"targetGroups,omitempty"`
	ProductionListenerARN string                        `json:"productionListenerArn,omitempty"`
	TestListenerARN       string                        `json:"testListenerArn,omitempty"`
}

type CodeDeployECSServiceBinding struct {
	ClusterName string `json:"clusterName,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
}

type ListenerInspection struct {
	ListenerARN     string                     `json:"listenerArn"`
	LoadBalancerARN string                     `json:"loadBalancerArn,omitempty"`
	Port            int32                      `json:"port,omitempty"`
	Protocol        string                     `json:"protocol,omitempty"`
	SSLPolicy       string                     `json:"sslPolicy,omitempty"`
	Certificates    []string                   `json:"certificates,omitempty"`
	DefaultActions  []ListenerActionInspection `json:"defaultActions,omitempty"`
}

type ListenerRuleInspection struct {
	RuleARN    string                            `json:"ruleArn"`
	Priority   string                            `json:"priority,omitempty"`
	IsDefault  bool                              `json:"isDefault"`
	Conditions []ListenerRuleConditionInspection `json:"conditions,omitempty"`
	Actions    []ListenerActionInspection        `json:"actions,omitempty"`
}

type ListenerRuleConditionInspection struct {
	Field  string   `json:"field,omitempty"`
	Values []string `json:"values,omitempty"`
}

type ListenerActionInspection struct {
	Type           string                           `json:"type,omitempty"`
	Order          int32                            `json:"order,omitempty"`
	TargetGroupARN string                           `json:"targetGroupArn,omitempty"`
	TargetGroups   []ListenerActionTargetInspection `json:"targetGroups,omitempty"`
}

type ListenerActionTargetInspection struct {
	TargetGroupARN string `json:"targetGroupArn,omitempty"`
	Weight         int32  `json:"weight,omitempty"`
}

type ECSServiceInspection struct {
	ClusterARN                    string                 `json:"clusterArn"`
	ServiceARN                    string                 `json:"serviceArn"`
	ServiceName                   string                 `json:"serviceName"`
	Status                        string                 `json:"status"`
	LaunchType                    string                 `json:"launchType,omitempty"`
	DesiredCount                  int32                  `json:"desiredCount"`
	RunningCount                  int32                  `json:"runningCount"`
	PendingCount                  int32                  `json:"pendingCount"`
	MinimumHealthyPercent         *int32                 `json:"minimumHealthyPercent,omitempty"`
	MaximumPercent                *int32                 `json:"maximumPercent,omitempty"`
	HealthCheckGracePeriodSeconds *int32                 `json:"healthCheckGracePeriodSeconds,omitempty"`
	TaskDefinition                string                 `json:"taskDefinition"`
	TaskDefinitionImage           string                 `json:"taskDefinitionImage,omitempty"`
	Deployments                   []ECSDeploymentSummary `json:"deployments"`
	Events                        []ECSServiceEvent      `json:"events"`
	Tasks                         []ECSTaskSummary       `json:"tasks,omitempty"`
	TasksScope                    *ECSTaskSummaryScope   `json:"tasksScope,omitempty"`
}

type ECSTaskSummaryScope struct {
	Available         bool   `json:"available"`
	RequestedLimit    *int   `json:"requestedLimit,omitempty"`
	EffectiveLimit    int    `json:"effectiveLimit"`
	ReturnedTaskCount int    `json:"returnedTaskCount"`
	TruncationStatus  string `json:"truncationStatus"`
	UnavailableReason string `json:"unavailableReason,omitempty"`
	Summary           string `json:"summary,omitempty"`
}

type ECSTaskSummary struct {
	TaskARN              string                 `json:"taskArn"`
	TaskID               string                 `json:"taskId"`
	ClusterARN           string                 `json:"clusterArn,omitempty"`
	ContainerInstanceARN string                 `json:"containerInstanceArn,omitempty"`
	EC2InstanceID        string                 `json:"ec2InstanceId,omitempty"`
	EC2Instance          *EC2InstanceDetail     `json:"ec2Instance,omitempty"`
	CapacityProviderName string                 `json:"capacityProviderName,omitempty"`
	AvailabilityZone     string                 `json:"availabilityZone,omitempty"`
	LastStatus           string                 `json:"lastStatus,omitempty"`
	DesiredStatus        string                 `json:"desiredStatus,omitempty"`
	HealthStatus         string                 `json:"healthStatus,omitempty"`
	LaunchType           string                 `json:"launchType,omitempty"`
	StartedAt            string                 `json:"startedAt,omitempty"`
	StoppedAt            string                 `json:"stoppedAt,omitempty"`
	StoppedReason        string                 `json:"stoppedReason,omitempty"`
	Containers           []ECSTaskContainerInfo `json:"containers,omitempty"`
}

type ECSTaskContainerInfo struct {
	Name         string       `json:"name"`
	Image        string       `json:"image,omitempty"`
	LastStatus   string       `json:"lastStatus,omitempty"`
	HealthStatus string       `json:"healthStatus,omitempty"`
	ExitCode     *int32       `json:"exitCode,omitempty"`
	Reason       string       `json:"reason,omitempty"`
	AWSLogs      *AWSLogsInfo `json:"awslogs,omitempty"`
}

type AWSLogsInfo struct {
	Group  string `json:"group"`
	Stream string `json:"stream"`
}

type ECSTaskInspection struct {
	ECSTaskSummary
	TaskDefinition string `json:"taskDefinition,omitempty"`
}

type ECSDeploymentSummary struct {
	ID                 string `json:"id,omitempty"`
	Status             string `json:"status,omitempty"`
	RolloutState       string `json:"rolloutState,omitempty"`
	RolloutStateReason string `json:"rolloutStateReason,omitempty"`
	TaskDefinition     string `json:"taskDefinition,omitempty"`
	DesiredCount       int32  `json:"desiredCount"`
	RunningCount       int32  `json:"runningCount"`
	PendingCount       int32  `json:"pendingCount"`
	CreatedAt          string `json:"createdAt,omitempty"`
	UpdatedAt          string `json:"updatedAt,omitempty"`
}

type ECSServiceEvent struct {
	CreatedAt string `json:"createdAt,omitempty"`
	Message   string `json:"message"`
}

type AutoScalingInspection struct {
	AutoScalingGroupARN           string                            `json:"autoScalingGroupArn"`
	AutoScalingGroupName          string                            `json:"autoScalingGroupName"`
	MinSize                       int32                             `json:"minSize"`
	MaxSize                       int32                             `json:"maxSize"`
	DesiredCapacity               int32                             `json:"desiredCapacity"`
	DefaultCooldown               int32                             `json:"defaultCooldown"`
	HealthCheckType               string                            `json:"healthCheckType,omitempty"`
	HealthCheckGracePeriod        int32                             `json:"healthCheckGracePeriod"`
	LaunchTemplate                *AutoScalingLaunchTemplateSummary `json:"launchTemplate,omitempty"`
	LaunchTemplateBootstrapSource string                            `json:"launchTemplateBootstrapSource,omitempty"`
	MixedInstancesPolicy          *AutoScalingMixedInstancesPolicy  `json:"mixedInstancesPolicy,omitempty"`
	TargetGroupARNs               []string                          `json:"targetGroupArns,omitempty"`
	Instances                     []AutoScalingGroupInstanceSummary `json:"instances"`
	InstancesScope                *AutoScalingInstanceScope         `json:"instancesScope,omitempty"`
}

type AutoScalingInstanceScope struct {
	Available             bool   `json:"available"`
	RequestedLimit        *int   `json:"requestedLimit,omitempty"`
	EffectiveLimit        int    `json:"effectiveLimit"`
	ReturnedInstanceCount int    `json:"returnedInstanceCount"`
	TruncationStatus      string `json:"truncationStatus"`
	Summary               string `json:"summary,omitempty"`
}

type AutoScalingLaunchTemplateSummary struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Version      string `json:"version,omitempty"`
	ImageID      string `json:"imageId,omitempty"`
	InstanceType string `json:"instanceType,omitempty"`
}

type AutoScalingMixedInstancesPolicy struct {
	LaunchTemplate *AutoScalingLaunchTemplateSummary   `json:"launchTemplate,omitempty"`
	Overrides      []AutoScalingLaunchTemplateOverride `json:"overrides,omitempty"`
}

type AutoScalingLaunchTemplateOverride struct {
	InstanceType     string                            `json:"instanceType,omitempty"`
	WeightedCapacity string                            `json:"weightedCapacity,omitempty"`
	LaunchTemplate   *AutoScalingLaunchTemplateSummary `json:"launchTemplate,omitempty"`
}

type AutoScalingGroupInstanceSummary struct {
	InstanceID           string             `json:"instanceId"`
	AvailabilityZone     string             `json:"availabilityZone,omitempty"`
	LifecycleState       string             `json:"lifecycleState,omitempty"`
	HealthStatus         string             `json:"healthStatus,omitempty"`
	ProtectedFromScaleIn bool               `json:"protectedFromScaleIn"`
	WeightedCapacity     string             `json:"weightedCapacity,omitempty"`
	EC2Instance          *EC2InstanceDetail `json:"ec2Instance,omitempty"`
}

type EC2InstanceDetail struct {
	InstanceState    string `json:"instanceState,omitempty"`
	InstanceType     string `json:"instanceType,omitempty"`
	ImageID          string `json:"imageId,omitempty"`
	PrivateIPAddress string `json:"privateIpAddress,omitempty"`
	PublicIPAddress  string `json:"publicIpAddress,omitempty"`
	SubnetID         string `json:"subnetId,omitempty"`
	VPCID            string `json:"vpcId,omitempty"`
	LaunchTime       string `json:"launchTime,omitempty"`
}

type Options struct {
	EnableAWS                     bool
	InspectECSService             func(context.Context, ECSRequest) (ECSServiceInspection, error)
	InspectECSTask                func(context.Context, ECSTaskRequest) (ECSTaskInspection, error)
	InspectAutoScalingGroup       func(context.Context, AutoScalingGroupRequest) (AutoScalingInspection, error)
	InspectTrafficRoutingResource func(context.Context, TrafficRoutingRequest) (TrafficRoutingInspection, error)
}

type Service struct {
	options             Options
	awsCache            *resourceaws.ClientCache
	ecsServiceHints     *ecsServiceClusterHintCache
	trafficRoutingCache *trafficRoutingInspectionCache
}

func NewService(options Options) Service {
	return Service{
		options:             options,
		awsCache:            resourceaws.NewClientCache(resourceaws.DefaultClientCacheDeps()),
		ecsServiceHints:     newECSServiceClusterHintCache(ecsServiceHintCacheCap),
		trafficRoutingCache: newTrafficRoutingInspectionCache(trafficRoutingCacheCap),
	}
}

type ecsServiceAPI interface {
	ListClusters(context.Context, *ecs.ListClustersInput, ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	ListTasks(context.Context, *ecs.ListTasksInput, ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
	DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	DescribeContainerInstances(context.Context, *ecs.DescribeContainerInstancesInput, ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error)
}

type ec2LaunchTemplateAPI interface {
	DescribeLaunchTemplateVersions(context.Context, *ec2.DescribeLaunchTemplateVersionsInput, ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplateVersionsOutput, error)
}

type ec2InstanceAPI interface {
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

type ecsServiceClusterHintKey struct {
	region          string
	targetAccountID string
	observerRoleARN string
	serviceName     string
}

type ecsServiceClusterHintEntry struct {
	key        ecsServiceClusterHintKey
	clusterARN string
}

type ecsServiceClusterHintCache struct {
	maxEntries int
	mu         sync.Mutex
	order      *list.List
	entries    map[ecsServiceClusterHintKey]*list.Element
}

type trafficRoutingInspectionCacheKey struct {
	region          string
	targetAccountID string
	observerRoleARN string
	resourceType    string
	resourceName    string
}

type trafficRoutingInspectionCacheEntry struct {
	key    trafficRoutingInspectionCacheKey
	detail TrafficRoutingInspection
}

type trafficRoutingInspectionCache struct {
	maxEntries int
	mu         sync.Mutex
	order      *list.List
	entries    map[trafficRoutingInspectionCacheKey]*list.Element
}

func newECSServiceClusterHintCache(maxEntries int) *ecsServiceClusterHintCache {
	if maxEntries <= 0 {
		maxEntries = ecsServiceHintCacheCap
	}
	return &ecsServiceClusterHintCache{
		maxEntries: maxEntries,
		order:      list.New(),
		entries:    make(map[ecsServiceClusterHintKey]*list.Element),
	}
}

func newTrafficRoutingInspectionCache(maxEntries int) *trafficRoutingInspectionCache {
	if maxEntries <= 0 {
		maxEntries = trafficRoutingCacheCap
	}
	return &trafficRoutingInspectionCache{
		maxEntries: maxEntries,
		order:      list.New(),
		entries:    make(map[trafficRoutingInspectionCacheKey]*list.Element),
	}
}

func newECSServiceClusterHintKey(region, targetAccountID, observerRoleARN, serviceName string) ecsServiceClusterHintKey {
	return ecsServiceClusterHintKey{
		region:          strings.TrimSpace(region),
		targetAccountID: strings.TrimSpace(targetAccountID),
		observerRoleARN: strings.TrimSpace(observerRoleARN),
		serviceName:     strings.TrimSpace(serviceName),
	}
}

func newTrafficRoutingInspectionCacheKey(region, targetAccountID, observerRoleARN, resourceType, resourceName string) trafficRoutingInspectionCacheKey {
	return trafficRoutingInspectionCacheKey{
		region:          strings.TrimSpace(region),
		targetAccountID: strings.TrimSpace(targetAccountID),
		observerRoleARN: strings.TrimSpace(observerRoleARN),
		resourceType:    strings.TrimSpace(resourceType),
		resourceName:    strings.TrimSpace(resourceName),
	}
}

func (c *ecsServiceClusterHintCache) Get(key ecsServiceClusterHintKey) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return "", false
	}
	c.order.MoveToFront(element)
	entry := element.Value.(ecsServiceClusterHintEntry)
	return entry.clusterARN, true
}

func (c *ecsServiceClusterHintCache) Set(key ecsServiceClusterHintKey, clusterARN string) {
	if c == nil {
		return
	}
	clusterARN = strings.TrimSpace(clusterARN)
	if clusterARN == "" {
		c.Delete(key)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		element.Value = ecsServiceClusterHintEntry{key: key, clusterARN: clusterARN}
		c.order.MoveToFront(element)
		return
	}
	element := c.order.PushFront(ecsServiceClusterHintEntry{key: key, clusterARN: clusterARN})
	c.entries[key] = element
	if c.order.Len() <= c.maxEntries {
		return
	}
	back := c.order.Back()
	if back == nil {
		return
	}
	entry := back.Value.(ecsServiceClusterHintEntry)
	delete(c.entries, entry.key)
	c.order.Remove(back)
}

func (c *trafficRoutingInspectionCache) Get(key trafficRoutingInspectionCacheKey) (TrafficRoutingInspection, bool) {
	if c == nil {
		return TrafficRoutingInspection{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return TrafficRoutingInspection{}, false
	}
	c.order.MoveToFront(element)
	entry := element.Value.(trafficRoutingInspectionCacheEntry)
	return cloneTrafficRoutingInspection(entry.detail), true
}

func (c *trafficRoutingInspectionCache) Set(key trafficRoutingInspectionCacheKey, detail TrafficRoutingInspection) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cloned := cloneTrafficRoutingInspection(detail)
	if element, ok := c.entries[key]; ok {
		element.Value = trafficRoutingInspectionCacheEntry{key: key, detail: cloned}
		c.order.MoveToFront(element)
		return
	}
	element := c.order.PushFront(trafficRoutingInspectionCacheEntry{key: key, detail: cloned})
	c.entries[key] = element
	if c.order.Len() <= c.maxEntries {
		return
	}
	back := c.order.Back()
	if back == nil {
		return
	}
	entry := back.Value.(trafficRoutingInspectionCacheEntry)
	delete(c.entries, entry.key)
	c.order.Remove(back)
}

func (c *ecsServiceClusterHintCache) Delete(key ecsServiceClusterHintKey) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return
	}
	delete(c.entries, key)
	c.order.Remove(element)
}

type ECSRequest struct {
	DeploymentName   string
	ResourceName     string
	TaskSummaryLimit int
	ClusterARN       string
	Region           string
	TargetAccountID  string
	ObserverRoleARN  string
}

type ECSTaskRequest struct {
	DeploymentName  string
	ResourceName    string
	ClusterARN      string
	Region          string
	TargetAccountID string
	ObserverRoleARN string
}

type AutoScalingGroupRequest struct {
	DeploymentName  string
	ResourceName    string
	InstanceLimit   int
	InstanceID      string
	Region          string
	TargetAccountID string
	ObserverRoleARN string
}

type TrafficRoutingRequest struct {
	DeploymentName      string
	ResourceType        string
	ResourceName        string
	ResourceDescription string
	Region              string
	TargetAccountID     string
	ObserverRoleARN     string
}

func (s Service) Inspect(ctx context.Context, req Request) (Result, error) {
	switch req.ResourceType {
	case "trafficShiftPlan":
		return availableTrafficRoutingResult(req, trafficRoutingInspection(req)), nil
	case "codeDeployApplication", "codeDeployDeploymentGroup", "productionListener", "testListener", "productionListenerRule", "blueTaskSetTargetGroup", "greenTaskSetTargetGroup", "activeTargetGroup", "candidateTargetGroup":
		if req.ResourceType == "productionListenerRule" && !strings.Contains(strings.TrimSpace(req.ResourceName), ":listener-rule/") {
			return availableTrafficRoutingResult(req, trafficRoutingInspection(req)), nil
		}
		cacheKey := newTrafficRoutingInspectionCacheKey(req.Region, req.TargetAccountID, req.ObserverRoleARN, req.ResourceType, req.ResourceName)
		if detail, ok := s.trafficRoutingCache.Get(cacheKey); ok {
			detail.Summary = trafficRoutingSummary(req)
			return availableTrafficRoutingResult(req, detail), nil
		}
		inspect := s.options.InspectTrafficRoutingResource
		if inspect == nil {
			if !s.options.EnableAWS {
				return availableTrafficRoutingResult(req, trafficRoutingInspection(req)), nil
			}
			inspect = s.inspectTrafficRoutingResourceFromAWS
		}
		if crossAccountWithoutObserverRole(req) {
			return unavailableResult(req, resourceaws.UnavailableReasonCrossAccountNotConfigured), nil
		}
		detail, err := inspect(ctx, TrafficRoutingRequest{
			DeploymentName:      req.DeploymentName,
			ResourceType:        req.ResourceType,
			ResourceName:        req.ResourceName,
			ResourceDescription: req.ResourceDescription,
			Region:              req.Region,
			TargetAccountID:     req.TargetAccountID,
			ObserverRoleARN:     req.ObserverRoleARN,
		})
		if err != nil {
			if reason, ok := resourceaws.UnavailableReasonForError(err); ok {
				if reason == resourceaws.UnavailableReasonAWSUnavailable {
					return availableTrafficRoutingResult(req, trafficRoutingInspection(req)), nil
				}
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		if detail.InspectionMode == "live" {
			s.trafficRoutingCache.Set(cacheKey, detail)
		}
		return availableTrafficRoutingResult(req, detail), nil
	case "ecsService":
		if crossAccountWithoutObserverRole(req) {
			return unavailableResult(req, resourceaws.UnavailableReasonCrossAccountNotConfigured), nil
		}
		inspect := s.options.InspectECSService
		if inspect == nil {
			if !s.options.EnableAWS {
				return Result{}, ErrNotImplemented
			}
			inspect = s.inspectECSServiceFromAWS
		}
		detail, err := inspect(ctx, ECSRequest{
			DeploymentName:   req.DeploymentName,
			ResourceName:     req.ResourceName,
			TaskSummaryLimit: req.TaskSummaryLimit,
			ClusterARN:       strings.TrimSpace(req.ClusterARN),
			Region:           req.Region,
			TargetAccountID:  req.TargetAccountID,
			ObserverRoleARN:  req.ObserverRoleARN,
		})
		if err != nil {
			if reason, ok := resourceaws.UnavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		return availableECSResult(req, detail), nil
	case "ecsTask":
		if crossAccountWithoutObserverRole(req) {
			return unavailableResult(req, resourceaws.UnavailableReasonCrossAccountNotConfigured), nil
		}
		inspect := s.options.InspectECSTask
		if inspect == nil {
			if !s.options.EnableAWS {
				return Result{}, ErrNotImplemented
			}
			inspect = s.inspectECSTaskFromAWS
		}
		detail, err := inspect(ctx, ECSTaskRequest{
			DeploymentName:  req.DeploymentName,
			ResourceName:    req.ResourceName,
			ClusterARN:      strings.TrimSpace(req.ClusterARN),
			Region:          req.Region,
			TargetAccountID: req.TargetAccountID,
			ObserverRoleARN: req.ObserverRoleARN,
		})
		if err != nil {
			if reason, ok := resourceaws.UnavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		return availableECSTaskResult(req, detail), nil
	case "candidateAutoScalingGroup", "serviceAutoScalingGroup":
		if crossAccountWithoutObserverRole(req) {
			return unavailableResult(req, resourceaws.UnavailableReasonCrossAccountNotConfigured), nil
		}
		inspect := s.options.InspectAutoScalingGroup
		if inspect == nil {
			if !s.options.EnableAWS {
				return Result{}, ErrNotImplemented
			}
			inspect = s.inspectAutoScalingGroupFromAWS
		}
		detail, err := inspect(ctx, AutoScalingGroupRequest{
			DeploymentName:  req.DeploymentName,
			ResourceName:    req.ResourceName,
			InstanceLimit:   req.AutoScalingInstanceLimit,
			InstanceID:      strings.TrimSpace(req.InstanceID),
			Region:          req.Region,
			TargetAccountID: req.TargetAccountID,
			ObserverRoleARN: req.ObserverRoleARN,
		})
		if err != nil {
			if reason, ok := resourceaws.UnavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		if instanceID := strings.TrimSpace(req.InstanceID); instanceID != "" {
			instance, ok := selectedAutoScalingInstance(detail, instanceID)
			if !ok {
				return unavailableResult(req, resourceaws.UnavailableReasonResourceNotFound), nil
			}
			detail.Instances = []AutoScalingGroupInstanceSummary{instance}
		}
		return availableAutoScalingResult(req, detail), nil
	case "userDataBootstrapSource":
		if crossAccountWithoutObserverRole(req) {
			return unavailableResult(req, resourceaws.UnavailableReasonCrossAccountNotConfigured), nil
		}
		if strings.TrimSpace(req.BackingResourceName) == "" {
			return unavailableResult(req, resourceaws.UnavailableReasonNoLogSource), nil
		}
		inspect := s.options.InspectAutoScalingGroup
		if inspect == nil {
			if !s.options.EnableAWS {
				return Result{}, ErrNotImplemented
			}
			inspect = s.inspectAutoScalingGroupFromAWS
		}
		detail, err := inspect(ctx, AutoScalingGroupRequest{
			DeploymentName:  req.DeploymentName,
			ResourceName:    req.BackingResourceName,
			InstanceID:      strings.TrimSpace(req.InstanceID),
			Region:          req.Region,
			TargetAccountID: req.TargetAccountID,
			ObserverRoleARN: req.ObserverRoleARN,
		})
		if err != nil {
			if reason, ok := resourceaws.UnavailableReasonForError(err); ok {
				return unavailableResult(req, reason), nil
			}
			return Result{}, err
		}
		if instanceID := strings.TrimSpace(req.InstanceID); instanceID != "" {
			if _, ok := selectedAutoScalingInstance(detail, instanceID); !ok {
				return unavailableResult(req, resourceaws.UnavailableReasonResourceNotFound), nil
			}
		}
		return availableUserDataBootstrapSourceResult(req, detail), nil
	default:
		return unavailableResult(req, resourceaws.UnavailableReasonUnsupportedResourceType), nil
	}
}

func crossAccountWithoutObserverRole(req Request) bool {
	return req.ControlAccountID != "" && req.TargetAccountID != "" && req.ControlAccountID != req.TargetAccountID && strings.TrimSpace(req.ObserverRoleARN) == ""
}

func (s Service) awsClients(ctx context.Context, region, targetAccountID, observerRoleARN string) (resourceaws.Clients, error) {
	cache := s.awsCache
	if cache == nil {
		cache = resourceaws.NewClientCache(resourceaws.DefaultClientCacheDeps())
	}
	return cache.Clients(ctx, region, targetAccountID, observerRoleARN)
}

func unavailableResult(req Request, reason string) Result {
	return Result{
		DeploymentName:      req.DeploymentName,
		ResourceType:        req.ResourceType,
		ResourceName:        req.ResourceName,
		ResourceDescription: strings.TrimSpace(req.ResourceDescription),
		InstanceID:          strings.TrimSpace(req.InstanceID),
		ClusterARN:          responseClusterARN(req),
		BackingResourceType: strings.TrimSpace(req.BackingResourceType),
		BackingResourceName: strings.TrimSpace(req.BackingResourceName),
		LookupContext:       lookupContext(req),
		Available:           false,
		UnavailableReason:   reason,
	}
}

func availableECSResult(req Request, detail ECSServiceInspection) Result {
	return Result{
		DeploymentName:      req.DeploymentName,
		ResourceType:        req.ResourceType,
		ResourceName:        req.ResourceName,
		ResourceDescription: strings.TrimSpace(req.ResourceDescription),
		ClusterARN:          responseClusterARN(req),
		LookupContext:       lookupContext(req),
		Available:           true,
		Source:              SourceECSService,
		ECSService:          &detail,
		ResultScope:         resultScopeFromECSTasks(detail.TasksScope),
		UnavailableReason:   "",
	}
}

func responseClusterARN(req Request) string {
	if req.ResourceType != "ecsService" && req.ResourceType != "ecsTask" {
		return ""
	}
	return strings.TrimSpace(req.ClusterARN)
}

func availableECSTaskResult(req Request, detail ECSTaskInspection) Result {
	return Result{
		DeploymentName:      req.DeploymentName,
		ResourceType:        req.ResourceType,
		ResourceName:        req.ResourceName,
		ResourceDescription: strings.TrimSpace(req.ResourceDescription),
		ClusterARN:          responseClusterARN(req),
		LookupContext:       lookupContext(req),
		Available:           true,
		Source:              SourceECSService,
		ECSTask:             &detail,
		UnavailableReason:   "",
	}
}

func availableAutoScalingResult(req Request, detail AutoScalingInspection) Result {
	instanceLimit := req.AutoScalingInstanceLimit
	if strings.TrimSpace(req.InstanceID) != "" {
		instanceLimit = 1
	}
	detail = applyAutoScalingInstanceScope(detail, instanceLimit)
	return Result{
		DeploymentName:      req.DeploymentName,
		ResourceType:        req.ResourceType,
		ResourceName:        req.ResourceName,
		ResourceDescription: strings.TrimSpace(req.ResourceDescription),
		InstanceID:          strings.TrimSpace(req.InstanceID),
		LookupContext:       lookupContext(req),
		Available:           true,
		Source:              SourceAutoScalingGroup,
		AutoScalingGroup:    &detail,
		ResultScope:         resultScopeFromAutoScalingInstances(detail.InstancesScope),
		UnavailableReason:   "",
	}
}

func resultScopeFromECSTasks(scope *ECSTaskSummaryScope) *ResultScope {
	if scope == nil {
		return nil
	}
	return &ResultScope{
		SampleType:        "ecsTasks",
		RequestedLimit:    cloneIntPtr(scope.RequestedLimit),
		EffectiveLimit:    scope.EffectiveLimit,
		ReturnedItemCount: scope.ReturnedTaskCount,
		ReturnedCount:     scope.ReturnedTaskCount,
		TruncationStatus:  scope.TruncationStatus,
		Truncated:         scope.TruncationStatus == "truncated",
	}
}

func resultScopeFromAutoScalingInstances(scope *AutoScalingInstanceScope) *ResultScope {
	if scope == nil {
		return nil
	}
	return &ResultScope{
		SampleType:        "autoScalingInstances",
		RequestedLimit:    cloneIntPtr(scope.RequestedLimit),
		EffectiveLimit:    scope.EffectiveLimit,
		ReturnedItemCount: scope.ReturnedInstanceCount,
		ReturnedCount:     scope.ReturnedInstanceCount,
		TruncationStatus:  scope.TruncationStatus,
		Truncated:         scope.TruncationStatus == "truncated",
	}
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func selectedAutoScalingInstance(detail AutoScalingInspection, instanceID string) (AutoScalingGroupInstanceSummary, bool) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return AutoScalingGroupInstanceSummary{}, false
	}
	for _, instance := range detail.Instances {
		if strings.TrimSpace(instance.InstanceID) == instanceID {
			return instance, true
		}
	}
	return AutoScalingGroupInstanceSummary{}, false
}

func enrichSelectedAutoScalingInstanceFromEC2(ctx context.Context, client ec2InstanceAPI, detail AutoScalingInspection, instanceID string) AutoScalingInspection {
	instanceID = strings.TrimSpace(instanceID)
	if client == nil || instanceID == "" {
		return detail
	}
	if _, ok := selectedAutoScalingInstance(detail, instanceID); !ok {
		return detail
	}
	ec2Detail, err := describeEC2InstanceDetail(ctx, client, instanceID)
	if err != nil || ec2Detail == nil {
		return detail
	}
	for idx := range detail.Instances {
		if strings.TrimSpace(detail.Instances[idx].InstanceID) == instanceID {
			detail.Instances[idx].EC2Instance = ec2Detail
			break
		}
	}
	return detail
}

func describeEC2InstanceDetail(ctx context.Context, client ec2InstanceAPI, instanceID string) (*EC2InstanceDetail, error) {
	instanceID = strings.TrimSpace(instanceID)
	if client == nil || instanceID == "" {
		return nil, nil
	}
	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	if err != nil {
		return nil, err
	}
	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			if strings.TrimSpace(aws.ToString(instance.InstanceId)) != instanceID {
				continue
			}
			detail := ec2InstanceDetailFromInstance(instance)
			return &detail, nil
		}
	}
	return nil, nil
}

func ec2InstanceDetailFromInstance(instance ec2types.Instance) EC2InstanceDetail {
	detail := EC2InstanceDetail{
		InstanceType:     strings.TrimSpace(string(instance.InstanceType)),
		ImageID:          strings.TrimSpace(aws.ToString(instance.ImageId)),
		PrivateIPAddress: strings.TrimSpace(aws.ToString(instance.PrivateIpAddress)),
		PublicIPAddress:  strings.TrimSpace(aws.ToString(instance.PublicIpAddress)),
		SubnetID:         strings.TrimSpace(aws.ToString(instance.SubnetId)),
		VPCID:            strings.TrimSpace(aws.ToString(instance.VpcId)),
		LaunchTime:       formatTime(aws.ToTime(instance.LaunchTime)),
	}
	if instance.State != nil {
		detail.InstanceState = strings.TrimSpace(string(instance.State.Name))
	}
	return detail
}

func applyAutoScalingInstanceScope(detail AutoScalingInspection, limit int) AutoScalingInspection {
	effectiveLimit := maxAutoScalingGroupInstanceSummaries
	var requestedLimit *int
	if limit > 0 {
		effectiveLimit = limit
		requested := limit
		requestedLimit = &requested
	}

	instances := append([]AutoScalingGroupInstanceSummary(nil), detail.Instances...)
	sort.SliceStable(instances, func(i, j int) bool {
		if instances[i].InstanceID == instances[j].InstanceID {
			if instances[i].AvailabilityZone == instances[j].AvailabilityZone {
				return instances[i].LifecycleState < instances[j].LifecycleState
			}
			return instances[i].AvailabilityZone < instances[j].AvailabilityZone
		}
		return instances[i].InstanceID < instances[j].InstanceID
	})

	truncationStatus := "not_truncated"
	if len(instances) > effectiveLimit {
		truncationStatus = "truncated"
		instances = instances[:effectiveLimit]
	}
	detail.Instances = instances
	detail.InstancesScope = &AutoScalingInstanceScope{
		Available:             true,
		RequestedLimit:        requestedLimit,
		EffectiveLimit:        effectiveLimit,
		ReturnedInstanceCount: len(instances),
		TruncationStatus:      truncationStatus,
		Summary:               autoScalingInstanceScopeSummary(len(instances), effectiveLimit, truncationStatus),
	}
	return detail
}

func autoScalingInstanceScopeSummary(returnedCount, effectiveLimit int, truncationStatus string) string {
	if truncationStatus == "truncated" {
		return fmt.Sprintf("Showing %d Auto Scaling instance(s); additional instances were omitted by the effective limit %d.", returnedCount, effectiveLimit)
	}
	return fmt.Sprintf("Returned %d Auto Scaling instance(s) within the effective limit %d.", returnedCount, effectiveLimit)
}

func availableUserDataBootstrapSourceResult(req Request, detail AutoScalingInspection) Result {
	var selectedInstance *AutoScalingGroupInstanceSummary
	if instance, ok := selectedAutoScalingInstance(detail, req.InstanceID); ok {
		copy := instance
		selectedInstance = &copy
	}
	return Result{
		DeploymentName:      req.DeploymentName,
		ResourceType:        req.ResourceType,
		ResourceName:        req.ResourceName,
		ResourceDescription: strings.TrimSpace(req.ResourceDescription),
		InstanceID:          strings.TrimSpace(req.InstanceID),
		BackingResourceType: strings.TrimSpace(req.BackingResourceType),
		BackingResourceName: strings.TrimSpace(req.BackingResourceName),
		LookupContext:       lookupContext(req),
		Available:           true,
		Source:              SourceAutoScalingGroup,
		UserDataBootstrapSource: &UserDataBootstrapSourceInspection{
			DesiredBootstrapSource:  req.ResourceName,
			ObservedBootstrapSource: strings.TrimSpace(detail.LaunchTemplateBootstrapSource),
			BackingResourceType:     strings.TrimSpace(req.BackingResourceType),
			BackingResourceName:     strings.TrimSpace(req.BackingResourceName),
			LaunchTemplate:          autoScalingInspectionObservedLaunchTemplate(detail),
			SelectedInstance:        selectedInstance,
		},
		UnavailableReason: "",
	}
}

func availableTrafficRoutingResult(req Request, detail TrafficRoutingInspection) Result {
	source := SourceTrafficRoutingAWS
	if detail.InspectionMode == "summary_only" {
		source = SourceRolloutSummary
	}
	return Result{
		DeploymentName:         req.DeploymentName,
		ResourceType:           req.ResourceType,
		ResourceName:           req.ResourceName,
		ResourceDescription:    strings.TrimSpace(req.ResourceDescription),
		LookupContext:          lookupContext(req),
		Available:              true,
		Source:                 source,
		TrafficRoutingResource: &detail,
		UnavailableReason:      "",
	}
}

func autoScalingInspectionObservedLaunchTemplate(detail AutoScalingInspection) *AutoScalingLaunchTemplateSummary {
	if detail.LaunchTemplate != nil {
		return cloneAutoScalingLaunchTemplateSummary(detail.LaunchTemplate)
	}
	if detail.MixedInstancesPolicy != nil && detail.MixedInstancesPolicy.LaunchTemplate != nil {
		return cloneAutoScalingLaunchTemplateSummary(detail.MixedInstancesPolicy.LaunchTemplate)
	}
	return nil
}

func cloneAutoScalingLaunchTemplateSummary(template *AutoScalingLaunchTemplateSummary) *AutoScalingLaunchTemplateSummary {
	if template == nil {
		return nil
	}
	cloned := *template
	return &cloned
}

func trafficRoutingInspection(req Request) TrafficRoutingInspection {
	return TrafficRoutingInspection{
		Kind:           req.ResourceType,
		DisplayName:    trafficRoutingDisplayName(req.ResourceType),
		InspectionMode: "summary_only",
		Summary:        trafficRoutingSummary(req),
	}
}

func trafficRoutingDisplayName(resourceType string) string {
	switch resourceType {
	case "codeDeployApplication":
		return "CodeDeploy application"
	case "codeDeployDeploymentGroup":
		return "CodeDeploy deployment group"
	case "productionListener":
		return "Production listener"
	case "testListener":
		return "Test listener"
	case "productionListenerRule":
		return "Production listener rule"
	case "activeTargetGroup":
		return "Active target group"
	case "candidateTargetGroup":
		return "Candidate target group"
	case "blueTaskSetTargetGroup":
		return "Blue task set target group"
	case "greenTaskSetTargetGroup":
		return "Green task set target group"
	case "trafficShiftPlan":
		return "Traffic shift plan"
	default:
		return "Traffic routing resource"
	}
}

func trafficRoutingSummary(req Request) string {
	return trafficRoutingSummaryText(req.ResourceType, req.ResourceDescription)
}

func trafficRoutingSummaryText(resourceType, resourceDescription string) string {
	if summary := strings.TrimSpace(resourceDescription); summary != "" {
		return summary
	}
	switch resourceType {
	case "codeDeployApplication":
		return "CodeDeploy application Tenet coordinates for ECS traffic shifting."
	case "codeDeployDeploymentGroup":
		return "CodeDeploy deployment group Tenet targets for rollout."
	case "productionListener":
		return "Production listener Tenet uses while shifting live traffic."
	case "testListener":
		return "Test listener Tenet uses to validate replacement capacity before promotion."
	case "productionListenerRule":
		return "Production listener rule Tenet updates while shifting weighted production traffic."
	case "activeTargetGroup":
		return "Active target group currently receiving production traffic."
	case "candidateTargetGroup":
		return "Candidate target group Tenet warms before weighted traffic shifting."
	case "blueTaskSetTargetGroup":
		return "Blue task set target group CodeDeploy treats as the live ECS traffic destination."
	case "greenTaskSetTargetGroup":
		return "Green task set target group CodeDeploy warms before shifting ECS traffic."
	case "trafficShiftPlan":
		return "Traffic shift plan Tenet follows while promoting the candidate resource."
	default:
		return "Traffic routing summary is available for this resource."
	}
}

func cloneTrafficRoutingInspection(detail TrafficRoutingInspection) TrafficRoutingInspection {
	cloned := detail
	if detail.CodeDeployApplication != nil {
		application := *detail.CodeDeployApplication
		application.DeploymentGroups = append([]string(nil), detail.CodeDeployApplication.DeploymentGroups...)
		cloned.CodeDeployApplication = &application
	}
	if detail.CodeDeployDeploymentGroup != nil {
		group := *detail.CodeDeployDeploymentGroup
		group.ECSServices = append([]CodeDeployECSServiceBinding(nil), detail.CodeDeployDeploymentGroup.ECSServices...)
		group.TargetGroups = append([]string(nil), detail.CodeDeployDeploymentGroup.TargetGroups...)
		cloned.CodeDeployDeploymentGroup = &group
	}
	if detail.Listener != nil {
		listener := *detail.Listener
		listener.Certificates = append([]string(nil), detail.Listener.Certificates...)
		listener.DefaultActions = cloneListenerActions(detail.Listener.DefaultActions)
		cloned.Listener = &listener
	}
	if detail.ListenerRule != nil {
		rule := *detail.ListenerRule
		rule.Conditions = cloneListenerRuleConditions(detail.ListenerRule.Conditions)
		rule.Actions = cloneListenerActions(detail.ListenerRule.Actions)
		cloned.ListenerRule = &rule
	}
	if detail.TargetGroup != nil {
		targetGroup := *detail.TargetGroup
		targetGroup.LoadBalancerARNs = append([]string(nil), detail.TargetGroup.LoadBalancerARNs...)
		targetGroup.TargetHealthCounts = cloneStringIntMap(detail.TargetGroup.TargetHealthCounts)
		targetGroup.Targets = append([]TargetHealthInspection(nil), detail.TargetGroup.Targets...)
		cloned.TargetGroup = &targetGroup
	}
	return cloned
}

func cloneStringIntMap(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]int, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneListenerActions(actions []ListenerActionInspection) []ListenerActionInspection {
	if len(actions) == 0 {
		return nil
	}
	cloned := make([]ListenerActionInspection, len(actions))
	for i, action := range actions {
		cloned[i] = action
		cloned[i].TargetGroups = append([]ListenerActionTargetInspection(nil), action.TargetGroups...)
	}
	return cloned
}

func cloneListenerRuleConditions(conditions []ListenerRuleConditionInspection) []ListenerRuleConditionInspection {
	if len(conditions) == 0 {
		return nil
	}
	cloned := make([]ListenerRuleConditionInspection, len(conditions))
	for i, condition := range conditions {
		cloned[i] = condition
		cloned[i].Values = append([]string(nil), condition.Values...)
	}
	return cloned
}

func lookupContext(req Request) LookupContext {
	return resourceaws.Lookup(resourceaws.RequestContext{
		ControlAccountID: req.ControlAccountID,
		TargetAccountID:  req.TargetAccountID,
		Region:           req.Region,
		ObserverRoleARN:  req.ObserverRoleARN,
	})
}

func (s Service) inspectECSServiceFromAWS(ctx context.Context, req ECSRequest) (ECSServiceInspection, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return ECSServiceInspection{}, err
	}
	service, clusterARN, err := findECSService(
		ctx,
		clients.ECS,
		req.ResourceName,
		strings.TrimSpace(req.ClusterARN),
		s.ecsServiceHints,
		newECSServiceClusterHintKey(req.Region, req.TargetAccountID, req.ObserverRoleARN, req.ResourceName),
	)
	if err != nil {
		return ECSServiceInspection{}, err
	}
	if service == nil {
		return ECSServiceInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
	}
	taskDefinitionImage := ""
	if taskDefinitionARN := strings.TrimSpace(aws.ToString(service.TaskDefinition)); taskDefinitionARN != "" {
		out, err := clients.ECS.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
			TaskDefinition: aws.String(taskDefinitionARN),
		})
		if err == nil {
			taskDefinitionImage = ecsTaskDefinitionImage(out.TaskDefinition)
		}
	}
	tasks, tasksScope, _ := ecsServiceTaskSummaries(ctx, clients.ECS, clients.EC2, aws.ToString(clusterARN), aws.ToString(service.ServiceName), req.TaskSummaryLimit)
	return ecsInspectionFromService(*service, aws.ToString(clusterARN), taskDefinitionImage, tasks, tasksScope), nil
}

func (s Service) inspectECSTaskFromAWS(ctx context.Context, req ECSTaskRequest) (ECSTaskInspection, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return ECSTaskInspection{}, err
	}
	task, clusterARN, err := findECSTask(ctx, clients.ECS, req.ResourceName, strings.TrimSpace(req.ClusterARN))
	if err != nil {
		return ECSTaskInspection{}, err
	}
	if task == nil {
		return ECSTaskInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
	}
	definitions := map[string]ecstypes.TaskDefinition{}
	if arn := strings.TrimSpace(aws.ToString(task.TaskDefinitionArn)); arn != "" {
		out, err := clients.ECS.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(arn)})
		if err == nil && out.TaskDefinition != nil {
			definitions[arn] = *out.TaskDefinition
		}
	}
	containerInstanceEC2IDs := ecsContainerInstanceEC2IDs(ctx, clients.ECS, clusterARN, []string{aws.ToString(task.ContainerInstanceArn)})
	inspection := ecsTaskInspectionFromTask(*task, clusterARN, definitions, containerInstanceEC2IDs)
	return enrichECSTaskInspectionFromEC2(ctx, clients.EC2, inspection), nil
}

func findECSTask(ctx context.Context, client ecsTaskAPI, taskARN, selectedClusterARN string) (*ecstypes.Task, string, error) {
	taskARN = strings.TrimSpace(taskARN)
	if taskARN == "" {
		return nil, "", nil
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
				return nil, "", err
			}
			clusters = append(clusters, page.ClusterArns...)
		}
	}
	for _, cluster := range clusters {
		out, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: aws.String(cluster), Tasks: []string{taskARN}})
		if err != nil {
			return nil, "", err
		}
		for _, task := range out.Tasks {
			if aws.ToString(task.TaskArn) == taskARN || taskIDFromARN(aws.ToString(task.TaskArn)) == taskIDFromARN(taskARN) {
				copy := task
				return &copy, cluster, nil
			}
		}
	}
	return nil, "", nil
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

type ecsTaskAPI interface {
	ListClusters(context.Context, *ecs.ListClustersInput, ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
	DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	DescribeContainerInstances(context.Context, *ecs.DescribeContainerInstancesInput, ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error)
}

func ecsTaskInspectionFromTask(task ecstypes.Task, clusterARN string, definitions map[string]ecstypes.TaskDefinition, containerInstanceEC2IDs map[string]string) ECSTaskInspection {
	return ECSTaskInspection{ECSTaskSummary: ecsTaskSummaryFromTask(task, clusterARN, definitions, containerInstanceEC2IDs), TaskDefinition: strings.TrimSpace(aws.ToString(task.TaskDefinitionArn))}
}

func enrichECSTaskInspectionFromEC2(ctx context.Context, client ec2InstanceAPI, detail ECSTaskInspection) ECSTaskInspection {
	instanceID := strings.TrimSpace(detail.EC2InstanceID)
	if client == nil || instanceID == "" {
		return detail
	}
	detail.EC2InstanceID = instanceID
	ec2Detail, err := describeEC2InstanceDetail(ctx, client, instanceID)
	if err != nil || ec2Detail == nil {
		return detail
	}
	detail.EC2Instance = ec2Detail
	return detail
}

func ecsTaskSummaryFromTask(task ecstypes.Task, clusterARN string, definitions map[string]ecstypes.TaskDefinition, containerInstanceEC2IDs map[string]string) ECSTaskSummary {
	containerDefs := map[string]ecstypes.ContainerDefinition{}
	if def, ok := definitions[strings.TrimSpace(aws.ToString(task.TaskDefinitionArn))]; ok {
		for _, container := range def.ContainerDefinitions {
			containerDefs[aws.ToString(container.Name)] = container
		}
	}
	containers := make([]ECSTaskContainerInfo, 0, len(task.Containers))
	taskID := taskIDFromARN(aws.ToString(task.TaskArn))
	for _, container := range task.Containers {
		info := ECSTaskContainerInfo{Name: strings.TrimSpace(aws.ToString(container.Name)), Image: strings.TrimSpace(aws.ToString(container.Image)), LastStatus: strings.TrimSpace(aws.ToString(container.LastStatus)), HealthStatus: string(container.HealthStatus), Reason: strings.TrimSpace(aws.ToString(container.Reason))}
		if container.ExitCode != nil {
			info.ExitCode = cloneInt32(container.ExitCode)
		}
		if def, ok := containerDefs[aws.ToString(container.Name)]; ok {
			group, stream := awsLogsLocation(def, taskID)
			if group != "" && stream != "" {
				info.AWSLogs = &AWSLogsInfo{Group: group, Stream: stream}
			}
		}
		containers = append(containers, info)
	}
	sort.SliceStable(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	containerInstanceARN := strings.TrimSpace(aws.ToString(task.ContainerInstanceArn))
	return ECSTaskSummary{TaskARN: strings.TrimSpace(aws.ToString(task.TaskArn)), TaskID: taskID, ClusterARN: strings.TrimSpace(clusterARN), ContainerInstanceARN: containerInstanceARN, EC2InstanceID: strings.TrimSpace(containerInstanceEC2IDs[containerInstanceARN]), CapacityProviderName: strings.TrimSpace(aws.ToString(task.CapacityProviderName)), AvailabilityZone: strings.TrimSpace(aws.ToString(task.AvailabilityZone)), LastStatus: strings.TrimSpace(aws.ToString(task.LastStatus)), DesiredStatus: strings.TrimSpace(aws.ToString(task.DesiredStatus)), HealthStatus: string(task.HealthStatus), LaunchType: string(task.LaunchType), StartedAt: formatTime(aws.ToTime(task.StartedAt)), StoppedAt: formatTime(aws.ToTime(task.StoppedAt)), StoppedReason: strings.TrimSpace(aws.ToString(task.StoppedReason)), Containers: containers}
}

func ecsServiceTaskSummaries(ctx context.Context, client ecsServiceAPI, ec2Client ec2InstanceAPI, clusterARN, serviceName string, limit int) ([]ECSTaskSummary, *ECSTaskSummaryScope, error) {
	clusterARN = strings.TrimSpace(clusterARN)
	serviceName = strings.TrimSpace(serviceName)
	if client == nil || clusterARN == "" || serviceName == "" {
		return nil, nil, nil
	}

	effectiveLimit := maxECSServiceTaskSummaries
	var requestedLimit *int
	if limit > 0 {
		effectiveLimit = limit
		requested := limit
		requestedLimit = &requested
	}

	taskARNs := make([]string, 0, effectiveLimit+1)
	seen := make(map[string]struct{})
	listPageLimit := int32(effectiveLimit + 1)
	if listPageLimit < ecsServiceTaskListPageLimit {
		listPageLimit = ecsServiceTaskListPageLimit
	}
	if listPageLimit > 100 {
		listPageLimit = 100
	}
	for _, desiredStatus := range []ecstypes.DesiredStatus{ecstypes.DesiredStatusRunning, ecstypes.DesiredStatusStopped} {
		pages := ecs.NewListTasksPaginator(client, &ecs.ListTasksInput{
			Cluster:       aws.String(clusterARN),
			ServiceName:   aws.String(serviceName),
			DesiredStatus: desiredStatus,
			MaxResults:    aws.Int32(listPageLimit),
		})
		for pages.HasMorePages() {
			out, err := pages.NextPage(ctx)
			if err != nil {
				return nil, unavailableECSTaskSummaryScope(requestedLimit, effectiveLimit, err), nil
			}
			for _, taskARN := range out.TaskArns {
				taskARN = strings.TrimSpace(taskARN)
				if taskARN == "" {
					continue
				}
				if _, ok := seen[taskARN]; ok {
					continue
				}
				seen[taskARN] = struct{}{}
				taskARNs = append(taskARNs, taskARN)
			}
			if len(taskARNs) > effectiveLimit {
				break
			}
		}
	}
	if len(taskARNs) == 0 {
		return nil, &ECSTaskSummaryScope{Available: true, RequestedLimit: requestedLimit, EffectiveLimit: effectiveLimit, ReturnedTaskCount: 0, TruncationStatus: "not_truncated"}, nil
	}

	tasks := make([]ecstypes.Task, 0, len(taskARNs))
	for start := 0; start < len(taskARNs); start += 100 {
		end := start + 100
		if end > len(taskARNs) {
			end = len(taskARNs)
		}
		out, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: aws.String(clusterARN), Tasks: taskARNs[start:end]})
		if err != nil {
			return nil, unavailableECSTaskSummaryScope(requestedLimit, effectiveLimit, err), nil
		}
		tasks = append(tasks, out.Tasks...)
	}
	definitions := ecsTaskDefinitionsByARN(ctx, client, tasks)
	containerInstanceEC2IDs := ecsContainerInstanceEC2IDs(ctx, client, clusterARN, ecsTaskContainerInstanceARNs(tasks))
	summaries := make([]ECSTaskSummary, 0, len(tasks))
	for _, task := range tasks {
		summary := ecsTaskSummaryFromTask(task, clusterARN, definitions, containerInstanceEC2IDs)
		if summary.TaskARN == "" {
			continue
		}
		summaries = append(summaries, summary)
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		left := ecsTaskSummarySortTime(summaries[i])
		right := ecsTaskSummarySortTime(summaries[j])
		if left.Equal(right) {
			return summaries[i].TaskARN < summaries[j].TaskARN
		}
		return left.After(right)
	})
	truncationStatus := "not_truncated"
	if len(summaries) > effectiveLimit {
		truncationStatus = "truncated"
		summaries = summaries[:effectiveLimit]
	} else if len(summaries) == effectiveLimit {
		truncationStatus = "unknown"
	}
	summaries = enrichECSTaskSummariesFromEC2(ctx, ec2Client, summaries)
	scope := &ECSTaskSummaryScope{Available: true, RequestedLimit: requestedLimit, EffectiveLimit: effectiveLimit, ReturnedTaskCount: len(summaries), TruncationStatus: truncationStatus}
	return summaries, scope, nil
}

func enrichECSTaskSummariesFromEC2(ctx context.Context, client ec2InstanceAPI, summaries []ECSTaskSummary) []ECSTaskSummary {
	if client == nil || len(summaries) == 0 {
		return summaries
	}
	details := make(map[string]*EC2InstanceDetail)
	for idx := range summaries {
		instanceID := strings.TrimSpace(summaries[idx].EC2InstanceID)
		if instanceID == "" {
			continue
		}
		summaries[idx].EC2InstanceID = instanceID
		detail, ok := details[instanceID]
		if !ok {
			var err error
			detail, err = describeEC2InstanceDetail(ctx, client, instanceID)
			if err != nil {
				detail = nil
			}
			details[instanceID] = detail
		}
		if detail != nil {
			summaries[idx].EC2Instance = detail
		}
	}
	return summaries
}

func unavailableECSTaskSummaryScope(requestedLimit *int, effectiveLimit int, err error) *ECSTaskSummaryScope {
	reason, ok := resourceaws.UnavailableReasonForError(err)
	if !ok || strings.TrimSpace(reason) == "" {
		reason = resourceaws.UnavailableReasonAWSUnavailable
	}
	return &ECSTaskSummaryScope{
		Available:         false,
		RequestedLimit:    requestedLimit,
		EffectiveLimit:    effectiveLimit,
		ReturnedTaskCount: 0,
		TruncationStatus:  "not_applicable",
		UnavailableReason: reason,
		Summary:           ecsTaskSummaryUnavailableText(reason),
	}
}

func ecsTaskSummaryUnavailableText(reason string) string {
	switch reason {
	case resourceaws.UnavailableReasonCrossAccountNotConfigured:
		return "ECS task sampling is unavailable because cross-account observer-role access is not configured."
	case resourceaws.UnavailableReasonResourceNotFound:
		return "ECS task sampling is unavailable because the service or task bridge resource could not be found."
	case resourceaws.UnavailableReasonAWSUnavailable:
		return "ECS task sampling is unavailable because AWS access for the task bridge could not be completed."
	default:
		return "ECS task sampling is unavailable; inspect the lookup context before treating this as an empty task sample."
	}
}

func ecsTaskDefinitionsByARN(ctx context.Context, client ecsServiceAPI, tasks []ecstypes.Task) map[string]ecstypes.TaskDefinition {
	definitions := make(map[string]ecstypes.TaskDefinition)
	for _, task := range tasks {
		arn := strings.TrimSpace(aws.ToString(task.TaskDefinitionArn))
		if arn == "" {
			continue
		}
		if _, ok := definitions[arn]; ok {
			continue
		}
		out, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(arn)})
		if err != nil || out.TaskDefinition == nil {
			continue
		}
		definitions[arn] = *out.TaskDefinition
	}
	return definitions
}

func ecsTaskContainerInstanceARNs(tasks []ecstypes.Task) []string {
	arns := make([]string, 0, len(tasks))
	seen := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		arn := strings.TrimSpace(aws.ToString(task.ContainerInstanceArn))
		if arn == "" {
			continue
		}
		if _, ok := seen[arn]; ok {
			continue
		}
		seen[arn] = struct{}{}
		arns = append(arns, arn)
	}
	return arns
}

func ecsContainerInstanceEC2IDs(ctx context.Context, client interface {
	DescribeContainerInstances(context.Context, *ecs.DescribeContainerInstancesInput, ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error)
}, clusterARN string, containerInstanceARNs []string) map[string]string {
	clusterARN = strings.TrimSpace(clusterARN)
	if client == nil || clusterARN == "" || len(containerInstanceARNs) == 0 {
		return nil
	}
	result := make(map[string]string)
	for start := 0; start < len(containerInstanceARNs); start += 100 {
		end := start + 100
		if end > len(containerInstanceARNs) {
			end = len(containerInstanceARNs)
		}
		batch := make([]string, 0, end-start)
		seen := make(map[string]struct{}, end-start)
		for _, arn := range containerInstanceARNs[start:end] {
			arn = strings.TrimSpace(arn)
			if arn == "" {
				continue
			}
			if _, ok := seen[arn]; ok {
				continue
			}
			seen[arn] = struct{}{}
			batch = append(batch, arn)
		}
		if len(batch) == 0 {
			continue
		}
		out, err := client.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{Cluster: aws.String(clusterARN), ContainerInstances: batch})
		if err != nil {
			continue
		}
		for _, containerInstance := range out.ContainerInstances {
			arn := strings.TrimSpace(aws.ToString(containerInstance.ContainerInstanceArn))
			ec2InstanceID := strings.TrimSpace(aws.ToString(containerInstance.Ec2InstanceId))
			if arn == "" || ec2InstanceID == "" {
				continue
			}
			result[arn] = ec2InstanceID
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func ecsTaskSummarySortTime(summary ECSTaskSummary) time.Time {
	for _, value := range []string{summary.StoppedAt, summary.StartedAt} {
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func (s Service) inspectAutoScalingGroupFromAWS(ctx context.Context, req AutoScalingGroupRequest) (AutoScalingInspection, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return AutoScalingInspection{}, err
	}
	out, err := clients.ASG.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{req.ResourceName},
	})
	if err != nil {
		return AutoScalingInspection{}, err
	}
	if len(out.AutoScalingGroups) == 0 {
		return AutoScalingInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
	}
	inspection := autoScalingInspectionFromGroup(out.AutoScalingGroups[0])
	if observedLaunchTemplate, bootstrapSource, err := autoScalingObservedLaunchTemplateFromGroup(ctx, clients.EC2, out.AutoScalingGroups[0]); err == nil {
		if observedLaunchTemplate != nil {
			if inspection.LaunchTemplate != nil {
				inspection.LaunchTemplate = observedLaunchTemplate
			} else if inspection.MixedInstancesPolicy != nil && inspection.MixedInstancesPolicy.LaunchTemplate != nil {
				inspection.MixedInstancesPolicy.LaunchTemplate = observedLaunchTemplate
			}
		}
		inspection.LaunchTemplateBootstrapSource = bootstrapSource
	}
	if instanceID := strings.TrimSpace(req.InstanceID); instanceID != "" {
		inspection = enrichSelectedAutoScalingInstanceFromEC2(ctx, clients.EC2, inspection, instanceID)
	}
	return inspection, nil
}

func ecsInspectionFromService(service ecstypes.Service, clusterARN string, taskDefinitionImage string, tasks []ECSTaskSummary, tasksScope *ECSTaskSummaryScope) ECSServiceInspection {
	var minimumHealthyPercent *int32
	var maximumPercent *int32
	if service.DeploymentConfiguration != nil {
		minimumHealthyPercent = cloneInt32(service.DeploymentConfiguration.MinimumHealthyPercent)
		maximumPercent = cloneInt32(service.DeploymentConfiguration.MaximumPercent)
	}

	deployments := append([]ecstypes.Deployment(nil), service.Deployments...)
	sort.SliceStable(deployments, func(i, j int) bool {
		left := aws.ToTime(deployments[i].CreatedAt)
		right := aws.ToTime(deployments[j].CreatedAt)
		if left.Equal(right) {
			return aws.ToString(deployments[i].Id) < aws.ToString(deployments[j].Id)
		}
		return left.After(right)
	})
	deploymentSummaries := make([]ECSDeploymentSummary, 0, len(deployments))
	for _, deployment := range deployments {
		deploymentSummaries = append(deploymentSummaries, ECSDeploymentSummary{
			ID:                 aws.ToString(deployment.Id),
			Status:             aws.ToString(deployment.Status),
			RolloutState:       string(deployment.RolloutState),
			RolloutStateReason: aws.ToString(deployment.RolloutStateReason),
			TaskDefinition:     aws.ToString(deployment.TaskDefinition),
			DesiredCount:       deployment.DesiredCount,
			RunningCount:       deployment.RunningCount,
			PendingCount:       deployment.PendingCount,
			CreatedAt:          formatTime(aws.ToTime(deployment.CreatedAt)),
			UpdatedAt:          formatTime(aws.ToTime(deployment.UpdatedAt)),
		})
	}

	events := append([]ecstypes.ServiceEvent(nil), service.Events...)
	sort.SliceStable(events, func(i, j int) bool {
		left := aws.ToTime(events[i].CreatedAt)
		right := aws.ToTime(events[j].CreatedAt)
		if left.Equal(right) {
			return aws.ToString(events[i].Message) < aws.ToString(events[j].Message)
		}
		return left.After(right)
	})
	if len(events) > 10 {
		events = events[:10]
	}
	eventSummaries := make([]ECSServiceEvent, 0, len(events))
	for _, event := range events {
		message := strings.TrimSpace(aws.ToString(event.Message))
		if message == "" {
			continue
		}
		eventSummaries = append(eventSummaries, ECSServiceEvent{
			CreatedAt: formatTime(aws.ToTime(event.CreatedAt)),
			Message:   message,
		})
	}

	return ECSServiceInspection{
		ClusterARN:                    clusterARN,
		ServiceARN:                    aws.ToString(service.ServiceArn),
		ServiceName:                   aws.ToString(service.ServiceName),
		Status:                        aws.ToString(service.Status),
		LaunchType:                    string(service.LaunchType),
		DesiredCount:                  service.DesiredCount,
		RunningCount:                  service.RunningCount,
		PendingCount:                  service.PendingCount,
		MinimumHealthyPercent:         minimumHealthyPercent,
		MaximumPercent:                maximumPercent,
		HealthCheckGracePeriodSeconds: cloneInt32(service.HealthCheckGracePeriodSeconds),
		TaskDefinition:                aws.ToString(service.TaskDefinition),
		TaskDefinitionImage:           strings.TrimSpace(taskDefinitionImage),
		Deployments:                   deploymentSummaries,
		Events:                        eventSummaries,
		Tasks:                         append([]ECSTaskSummary(nil), tasks...),
		TasksScope:                    cloneECSTaskSummaryScope(tasksScope),
	}
}

func cloneECSTaskSummaryScope(scope *ECSTaskSummaryScope) *ECSTaskSummaryScope {
	if scope == nil {
		return nil
	}
	cloned := *scope
	return &cloned
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func ecsTaskDefinitionImage(taskDefinition *ecstypes.TaskDefinition) string {
	if taskDefinition == nil {
		return ""
	}
	images := make(map[string]struct{}, len(taskDefinition.ContainerDefinitions))
	for _, container := range taskDefinition.ContainerDefinitions {
		image := strings.TrimSpace(aws.ToString(container.Image))
		if image == "" {
			continue
		}
		images[image] = struct{}{}
	}
	if len(images) != 1 {
		return ""
	}
	for image := range images {
		return image
	}
	return ""
}

func autoScalingInspectionFromGroup(group asgtypes.AutoScalingGroup) AutoScalingInspection {
	targetGroupARNs := append([]string(nil), group.TargetGroupARNs...)
	sort.Strings(targetGroupARNs)

	instances := append([]asgtypes.Instance(nil), group.Instances...)
	sort.SliceStable(instances, func(i, j int) bool {
		return aws.ToString(instances[i].InstanceId) < aws.ToString(instances[j].InstanceId)
	})

	instanceSummaries := make([]AutoScalingGroupInstanceSummary, 0, len(instances))
	for _, instance := range instances {
		instanceSummaries = append(instanceSummaries, AutoScalingGroupInstanceSummary{
			InstanceID:           aws.ToString(instance.InstanceId),
			AvailabilityZone:     aws.ToString(instance.AvailabilityZone),
			LifecycleState:       string(instance.LifecycleState),
			HealthStatus:         aws.ToString(instance.HealthStatus),
			ProtectedFromScaleIn: aws.ToBool(instance.ProtectedFromScaleIn),
			WeightedCapacity:     aws.ToString(instance.WeightedCapacity),
		})
	}

	return AutoScalingInspection{
		AutoScalingGroupARN:    aws.ToString(group.AutoScalingGroupARN),
		AutoScalingGroupName:   aws.ToString(group.AutoScalingGroupName),
		MinSize:                aws.ToInt32(group.MinSize),
		MaxSize:                aws.ToInt32(group.MaxSize),
		DesiredCapacity:        aws.ToInt32(group.DesiredCapacity),
		DefaultCooldown:        aws.ToInt32(group.DefaultCooldown),
		HealthCheckType:        aws.ToString(group.HealthCheckType),
		HealthCheckGracePeriod: aws.ToInt32(group.HealthCheckGracePeriod),
		LaunchTemplate:         autoScalingLaunchTemplateSummary(group.LaunchTemplate),
		MixedInstancesPolicy:   autoScalingMixedInstancesPolicy(group.MixedInstancesPolicy),
		TargetGroupARNs:        targetGroupARNs,
		Instances:              instanceSummaries,
	}
}

func autoScalingPrimaryLaunchTemplateSpecFromGroup(group asgtypes.AutoScalingGroup) *asgtypes.LaunchTemplateSpecification {
	spec := group.LaunchTemplate
	if spec == nil && group.MixedInstancesPolicy != nil && group.MixedInstancesPolicy.LaunchTemplate != nil {
		spec = group.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification
	}
	return spec
}

func autoScalingObservedLaunchTemplateFromGroup(ctx context.Context, client ec2LaunchTemplateAPI, group asgtypes.AutoScalingGroup) (*AutoScalingLaunchTemplateSummary, string, error) {
	return autoScalingObservedLaunchTemplate(ctx, client, autoScalingPrimaryLaunchTemplateSpecFromGroup(group))
}

func autoScalingObservedLaunchTemplate(ctx context.Context, client ec2LaunchTemplateAPI, spec *asgtypes.LaunchTemplateSpecification) (*AutoScalingLaunchTemplateSummary, string, error) {
	summary := autoScalingLaunchTemplateSummary(spec)
	if client == nil || spec == nil {
		return summary, "", nil
	}
	input := &ec2.DescribeLaunchTemplateVersionsInput{}
	if launchTemplateID := strings.TrimSpace(aws.ToString(spec.LaunchTemplateId)); launchTemplateID != "" {
		input.LaunchTemplateId = aws.String(launchTemplateID)
	} else if launchTemplateName := strings.TrimSpace(aws.ToString(spec.LaunchTemplateName)); launchTemplateName != "" {
		input.LaunchTemplateName = aws.String(launchTemplateName)
	} else {
		return summary, "", nil
	}
	if version := strings.TrimSpace(aws.ToString(spec.Version)); version != "" {
		input.Versions = []string{version}
	}
	out, err := client.DescribeLaunchTemplateVersions(ctx, input)
	if err != nil {
		return nil, "", err
	}
	bootstrapSource := ""
	for _, version := range out.LaunchTemplateVersions {
		if version.LaunchTemplateData == nil {
			continue
		}
		if summary != nil {
			summary.ImageID = strings.TrimSpace(aws.ToString(version.LaunchTemplateData.ImageId))
			summary.InstanceType = strings.TrimSpace(string(version.LaunchTemplateData.InstanceType))
		}
		bootstrapSource = extractLaunchTemplateBootstrapSource(aws.ToString(version.LaunchTemplateData.UserData))
		break
	}
	return summary, bootstrapSource, nil
}

func extractLaunchTemplateBootstrapSource(encodedUserData string) string {
	encodedUserData = strings.TrimSpace(encodedUserData)
	if encodedUserData == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(encodedUserData)
	if err != nil {
		return ""
	}
	body := strings.TrimSpace(string(decoded))
	if body == "" {
		return ""
	}
	for _, pattern := range launchTemplateBootstrapSourcePatterns {
		matches := pattern.FindStringSubmatch(body)
		if len(matches) < 2 {
			continue
		}
		if source := strings.TrimSpace(matches[1]); source != "" {
			return source
		}
	}
	if !strings.ContainsAny(body, "\r\n\t ") && strings.Contains(body, ":") {
		return body
	}
	return ""
}

func autoScalingLaunchTemplateSummary(spec *asgtypes.LaunchTemplateSpecification) *AutoScalingLaunchTemplateSummary {
	if spec == nil {
		return nil
	}
	summary := &AutoScalingLaunchTemplateSummary{
		ID:      strings.TrimSpace(aws.ToString(spec.LaunchTemplateId)),
		Name:    strings.TrimSpace(aws.ToString(spec.LaunchTemplateName)),
		Version: strings.TrimSpace(aws.ToString(spec.Version)),
	}
	if summary.ID == "" && summary.Name == "" && summary.Version == "" {
		return nil
	}
	return summary
}

func autoScalingMixedInstancesPolicy(policy *asgtypes.MixedInstancesPolicy) *AutoScalingMixedInstancesPolicy {
	if policy == nil || policy.LaunchTemplate == nil {
		return nil
	}

	detail := &AutoScalingMixedInstancesPolicy{
		LaunchTemplate: autoScalingLaunchTemplateSummary(policy.LaunchTemplate.LaunchTemplateSpecification),
	}

	for _, override := range policy.LaunchTemplate.Overrides {
		mapped := AutoScalingLaunchTemplateOverride{
			InstanceType:     strings.TrimSpace(aws.ToString(override.InstanceType)),
			WeightedCapacity: strings.TrimSpace(aws.ToString(override.WeightedCapacity)),
			LaunchTemplate:   autoScalingLaunchTemplateSummary(override.LaunchTemplateSpecification),
		}
		if mapped.InstanceType == "" && mapped.WeightedCapacity == "" && mapped.LaunchTemplate == nil {
			continue
		}
		detail.Overrides = append(detail.Overrides, mapped)
	}

	if detail.LaunchTemplate == nil && len(detail.Overrides) == 0 {
		return nil
	}
	return detail
}

func findECSService(ctx context.Context, client ecsServiceAPI, serviceName, selectedClusterARN string, hints *ecsServiceClusterHintCache, key ecsServiceClusterHintKey) (*ecstypes.Service, *string, error) {
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

	skipClusterARN := ""
	if cachedClusterARN, ok := hints.Get(key); ok {
		service, err := describeECSServiceInCluster(ctx, client, cachedClusterARN, serviceName)
		if err != nil {
			return nil, nil, err
		}
		if service != nil {
			hints.Set(key, cachedClusterARN)
			return service, aws.String(cachedClusterARN), nil
		}
		hints.Delete(key)
		skipClusterARN = cachedClusterARN
	}

	var nextToken *string
	for {
		page, err := client.ListClusters(ctx, &ecs.ListClustersInput{NextToken: nextToken})
		if err != nil {
			return nil, nil, err
		}
		for _, clusterARN := range page.ClusterArns {
			if clusterARN == skipClusterARN {
				continue
			}
			service, err := describeECSServiceInCluster(ctx, client, clusterARN, serviceName)
			if err != nil {
				return nil, nil, err
			}
			if service == nil {
				continue
			}
			hints.Set(key, clusterARN)
			return service, aws.String(clusterARN), nil
		}
		if aws.ToString(page.NextToken) == "" {
			break
		}
		nextToken = page.NextToken
	}
	hints.Delete(key)
	return nil, nil, nil
}

func describeECSServiceInCluster(ctx context.Context, client ecsServiceAPI, clusterARN, serviceName string) (*ecstypes.Service, error) {
	out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterARN),
		Services: []string{serviceName},
	})
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

func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
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
	return groupName, streamPrefix + "/" + containerName + "/" + taskID
}

func taskIDFromARN(arn string) string {
	if arn == "" {
		return ""
	}
	parts := strings.Split(arn, "/")
	return parts[len(parts)-1]
}
