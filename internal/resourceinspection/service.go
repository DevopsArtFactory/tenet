package resourceinspection

import (
	"container/list"
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/DevopsArtFactory/tenet/internal/resourceaws"
)

const (
	SourceECSService       = "ecs_service"
	SourceAutoScalingGroup = "auto_scaling_group"
	ecsServiceHintCacheCap = 256
)

var ErrNotImplemented = errors.New("resource inspection service not implemented")

type LookupContext = resourceaws.LookupContext

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
	DeploymentName    string                 `json:"deploymentName"`
	ResourceType      string                 `json:"resourceType"`
	ResourceName      string                 `json:"resourceName"`
	LookupContext     LookupContext          `json:"lookupContext"`
	Available         bool                   `json:"available"`
	Source            string                 `json:"source"`
	ECSService        *ECSServiceInspection  `json:"ecsService,omitempty"`
	AutoScalingGroup  *AutoScalingInspection `json:"autoScalingGroup,omitempty"`
	UnavailableReason string                 `json:"unavailableReason"`
}

type ECSServiceInspection struct {
	ClusterARN     string                 `json:"clusterArn"`
	ServiceARN     string                 `json:"serviceArn"`
	ServiceName    string                 `json:"serviceName"`
	Status         string                 `json:"status"`
	LaunchType     string                 `json:"launchType,omitempty"`
	DesiredCount   int32                  `json:"desiredCount"`
	RunningCount   int32                  `json:"runningCount"`
	PendingCount   int32                  `json:"pendingCount"`
	TaskDefinition string                 `json:"taskDefinition"`
	Deployments    []ECSDeploymentSummary `json:"deployments"`
	Events         []ECSServiceEvent      `json:"events"`
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
	AutoScalingGroupARN    string                            `json:"autoScalingGroupArn"`
	AutoScalingGroupName   string                            `json:"autoScalingGroupName"`
	MinSize                int32                             `json:"minSize"`
	MaxSize                int32                             `json:"maxSize"`
	DesiredCapacity        int32                             `json:"desiredCapacity"`
	DefaultCooldown        int32                             `json:"defaultCooldown"`
	HealthCheckType        string                            `json:"healthCheckType,omitempty"`
	HealthCheckGracePeriod int32                             `json:"healthCheckGracePeriod"`
	TargetGroupARNs        []string                          `json:"targetGroupArns,omitempty"`
	Instances              []AutoScalingGroupInstanceSummary `json:"instances"`
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
	EnableAWS               bool
	InspectECSService       func(context.Context, ECSRequest) (ECSServiceInspection, error)
	InspectAutoScalingGroup func(context.Context, AutoScalingGroupRequest) (AutoScalingInspection, error)
}

type Service struct {
	options         Options
	awsCache        *resourceaws.ClientCache
	ecsServiceHints *ecsServiceClusterHintCache
}

func NewService(options Options) Service {
	return Service{
		options:         options,
		awsCache:        resourceaws.NewClientCache(resourceaws.DefaultClientCacheDeps()),
		ecsServiceHints: newECSServiceClusterHintCache(ecsServiceHintCacheCap),
	}
}

type ecsServiceAPI interface {
	ListClusters(context.Context, *ecs.ListClustersInput, ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
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

func newECSServiceClusterHintKey(region, targetAccountID, observerRoleARN, serviceName string) ecsServiceClusterHintKey {
	return ecsServiceClusterHintKey{
		region:          strings.TrimSpace(region),
		targetAccountID: strings.TrimSpace(targetAccountID),
		observerRoleARN: strings.TrimSpace(observerRoleARN),
		serviceName:     strings.TrimSpace(serviceName),
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

func (s Service) Inspect(ctx context.Context, req Request) (Result, error) {
	if req.ControlAccountID != "" && req.TargetAccountID != "" && req.ControlAccountID != req.TargetAccountID && strings.TrimSpace(req.ObserverRoleARN) == "" {
		return unavailableResult(req, resourceaws.UnavailableReasonCrossAccountNotConfigured), nil
	}

	switch req.ResourceType {
	case "ecsService":
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
	default:
		return unavailableResult(req, resourceaws.UnavailableReasonUnsupportedResourceType), nil
	}
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
	return ecsInspectionFromService(*service, aws.ToString(clusterARN)), nil
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
	return autoScalingInspectionFromGroup(out.AutoScalingGroups[0]), nil
}

func ecsInspectionFromService(service ecstypes.Service, clusterARN string) ECSServiceInspection {
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
		ClusterARN:     clusterARN,
		ServiceARN:     aws.ToString(service.ServiceArn),
		ServiceName:    aws.ToString(service.ServiceName),
		Status:         aws.ToString(service.Status),
		LaunchType:     string(service.LaunchType),
		DesiredCount:   service.DesiredCount,
		RunningCount:   service.RunningCount,
		PendingCount:   service.PendingCount,
		TaskDefinition: aws.ToString(service.TaskDefinition),
		Deployments:    deploymentSummaries,
		Events:         eventSummaries,
	}
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
		TargetGroupARNs:        targetGroupARNs,
		Instances:              instanceSummaries,
	}
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
