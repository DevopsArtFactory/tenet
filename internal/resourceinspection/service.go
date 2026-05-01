package resourceinspection

import (
	"container/list"
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/DevopsArtFactory/tenet/internal/resourceaws"
)

const (
	SourceECSService        = "ecs_service"
	SourceAutoScalingGroup  = "auto_scaling_group"
	SourceTrafficRoutingAWS = "traffic_routing_aws"
	SourceRolloutSummary    = "rollout_execution_summary"
	ecsServiceHintCacheCap  = 256
	trafficRoutingCacheCap  = 128
)

var launchTemplateBootstrapSourcePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^\s*(?:export\s+)?(?:TENET_)?(?:USERDATA|BOOTSTRAP)_SOURCE\s*=\s*['"]?([^'"#\s]+)['"]?`),
	regexp.MustCompile(`(?im)^\s*(?:#\s*)?(?:tenet[-_\s]*)?(?:userdata|bootstrap)[-_\s]*source\s*[:=]\s*['"]?([^'"#\s]+)['"]?`),
	regexp.MustCompile(`(?im)(?:--userdata-source|--bootstrap-source)\s+['"]?([^'"\s]+)`),
}

var ErrNotImplemented = errors.New("resource inspection service not implemented")

type LookupContext = resourceaws.LookupContext

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
}

type Result struct {
	DeploymentName          string                             `json:"deploymentName"`
	ResourceType            string                             `json:"resourceType"`
	ResourceName            string                             `json:"resourceName"`
	LookupContext           LookupContext                      `json:"lookupContext"`
	Available               bool                               `json:"available"`
	Source                  string                             `json:"source"`
	ECSService              *ECSServiceInspection              `json:"ecsService,omitempty"`
	AutoScalingGroup        *AutoScalingInspection             `json:"autoScalingGroup,omitempty"`
	UserDataBootstrapSource *UserDataBootstrapSourceInspection `json:"userDataBootstrapSource,omitempty"`
	TrafficRoutingResource  *TrafficRoutingInspection          `json:"trafficRoutingResource,omitempty"`
	UnavailableReason       string                             `json:"unavailableReason"`
}

type UserDataBootstrapSourceInspection struct {
	DesiredBootstrapSource  string                            `json:"desiredBootstrapSource"`
	ObservedBootstrapSource string                            `json:"observedBootstrapSource,omitempty"`
	BackingResourceType     string                            `json:"backingResourceType,omitempty"`
	BackingResourceName     string                            `json:"backingResourceName,omitempty"`
	LaunchTemplate          *AutoScalingLaunchTemplateSummary `json:"launchTemplate,omitempty"`
}

type TrafficRoutingInspection struct {
	Kind                      string                               `json:"kind"`
	DisplayName               string                               `json:"displayName"`
	InspectionMode            string                               `json:"inspectionMode"`
	Summary                   string                               `json:"summary"`
	CodeDeployApplication     *CodeDeployApplicationInspection     `json:"codeDeployApplication,omitempty"`
	CodeDeployDeploymentGroup *CodeDeployDeploymentGroupInspection `json:"codeDeployDeploymentGroup,omitempty"`
	Listener                  *ListenerInspection                  `json:"listener,omitempty"`
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
	InstanceID           string `json:"instanceId"`
	AvailabilityZone     string `json:"availabilityZone,omitempty"`
	LifecycleState       string `json:"lifecycleState,omitempty"`
	HealthStatus         string `json:"healthStatus,omitempty"`
	ProtectedFromScaleIn bool   `json:"protectedFromScaleIn"`
	WeightedCapacity     string `json:"weightedCapacity,omitempty"`
}

type Options struct {
	EnableAWS                     bool
	InspectECSService             func(context.Context, ECSRequest) (ECSServiceInspection, error)
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
}

type ec2LaunchTemplateAPI interface {
	DescribeLaunchTemplateVersions(context.Context, *ec2.DescribeLaunchTemplateVersionsInput, ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplateVersionsOutput, error)
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
	DeploymentName  string
	ResourceName    string
	Region          string
	TargetAccountID string
	ObserverRoleARN string
}

type AutoScalingGroupRequest struct {
	DeploymentName  string
	ResourceName    string
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
	case "trafficShiftPlan", "productionListenerRule":
		return availableTrafficRoutingResult(req, trafficRoutingInspection(req)), nil
	case "codeDeployApplication", "codeDeployDeploymentGroup", "productionListener", "testListener", "blueTaskSetTargetGroup", "greenTaskSetTargetGroup", "activeTargetGroup", "candidateTargetGroup":
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
			DeploymentName:  req.DeploymentName,
			ResourceName:    req.ResourceName,
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
		return availableECSResult(req, detail), nil
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
		DeploymentName:    req.DeploymentName,
		ResourceType:      req.ResourceType,
		ResourceName:      req.ResourceName,
		LookupContext:     lookupContext(req),
		Available:         false,
		UnavailableReason: reason,
	}
}

func availableECSResult(req Request, detail ECSServiceInspection) Result {
	return Result{
		DeploymentName:    req.DeploymentName,
		ResourceType:      req.ResourceType,
		ResourceName:      req.ResourceName,
		LookupContext:     lookupContext(req),
		Available:         true,
		Source:            SourceECSService,
		ECSService:        &detail,
		UnavailableReason: "",
	}
}

func availableAutoScalingResult(req Request, detail AutoScalingInspection) Result {
	return Result{
		DeploymentName:    req.DeploymentName,
		ResourceType:      req.ResourceType,
		ResourceName:      req.ResourceName,
		LookupContext:     lookupContext(req),
		Available:         true,
		Source:            SourceAutoScalingGroup,
		AutoScalingGroup:  &detail,
		UnavailableReason: "",
	}
}

func availableUserDataBootstrapSourceResult(req Request, detail AutoScalingInspection) Result {
	return Result{
		DeploymentName: req.DeploymentName,
		ResourceType:   req.ResourceType,
		ResourceName:   req.ResourceName,
		LookupContext:  lookupContext(req),
		Available:      true,
		Source:         SourceAutoScalingGroup,
		UserDataBootstrapSource: &UserDataBootstrapSourceInspection{
			DesiredBootstrapSource:  req.ResourceName,
			ObservedBootstrapSource: strings.TrimSpace(detail.LaunchTemplateBootstrapSource),
			BackingResourceType:     strings.TrimSpace(req.BackingResourceType),
			BackingResourceName:     strings.TrimSpace(req.BackingResourceName),
			LaunchTemplate:          autoScalingInspectionObservedLaunchTemplate(detail),
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
	return ecsInspectionFromService(*service, aws.ToString(clusterARN), taskDefinitionImage), nil
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
	return inspection, nil
}

func ecsInspectionFromService(service ecstypes.Service, clusterARN string, taskDefinitionImage string) ECSServiceInspection {
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
	}
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

func findECSService(ctx context.Context, client ecsServiceAPI, serviceName string, hints *ecsServiceClusterHintCache, key ecsServiceClusterHintKey) (*ecstypes.Service, *string, error) {
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
