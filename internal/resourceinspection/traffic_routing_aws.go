package resourceinspection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	"github.com/DevopsArtFactory/tenet/internal/resourceaws"
)

func (s Service) inspectTrafficRoutingResourceFromAWS(ctx context.Context, req TrafficRoutingRequest) (TrafficRoutingInspection, error) {
	clients, err := s.awsClients(ctx, req.Region, req.TargetAccountID, req.ObserverRoleARN)
	if err != nil {
		return TrafficRoutingInspection{}, err
	}
	switch req.ResourceType {
	case "codeDeployApplication":
		application, err := getCodeDeployApplication(ctx, clients.Config, req.Region, codeDeployApplicationName(req.DeploymentName, req.ResourceName, req.ResourceType))
		if err != nil {
			return TrafficRoutingInspection{}, err
		}
		return trafficRoutingInspectionFromApplication(req, application), nil
	case "activeTargetGroup", "candidateTargetGroup":
		targetGroupName := strings.TrimSpace(req.ResourceName)
		if targetGroupName == "" {
			return TrafficRoutingInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
		}
		targetGroup, err := describeTargetGroup(ctx, clients.Config, req.Region, targetGroupName)
		if err != nil {
			return TrafficRoutingInspection{}, err
		}
		targetHealth, err := describeTargetHealth(ctx, clients.Config, req.Region, targetGroup.TargetGroupARN)
		if err != nil {
			return TrafficRoutingInspection{}, err
		}
		return trafficRoutingInspectionFromTargetGroup(req, targetGroup, targetHealth), nil
	}
	group, err := getCodeDeployDeploymentGroup(ctx, clients.Config, req.Region, codeDeployApplicationName(req.DeploymentName, req.ResourceName, req.ResourceType), codeDeployDeploymentGroupName(req))
	if err != nil {
		return TrafficRoutingInspection{}, err
	}
	switch req.ResourceType {
	case "codeDeployDeploymentGroup":
		return trafficRoutingInspectionFromDeploymentGroup(req, group), nil
	case "productionListener":
		listenerARN := codeDeployProductionListenerARN(group)
		if listenerARN == "" {
			return TrafficRoutingInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
		}
		listener, err := describeListener(ctx, clients.Config, req.Region, listenerARN)
		if err != nil {
			return TrafficRoutingInspection{}, err
		}
		return trafficRoutingInspectionFromListener(req, listener), nil
	case "testListener":
		listenerARN := codeDeployTestListenerARN(group)
		if listenerARN == "" {
			return TrafficRoutingInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
		}
		listener, err := describeListener(ctx, clients.Config, req.Region, listenerARN)
		if err != nil {
			return TrafficRoutingInspection{}, err
		}
		return trafficRoutingInspectionFromListener(req, listener), nil
	case "blueTaskSetTargetGroup", "greenTaskSetTargetGroup":
		targetGroupName := codeDeployTaskSetTargetGroupName(group, req.ResourceType, req.ResourceName)
		if targetGroupName == "" {
			return TrafficRoutingInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
		}
		targetGroup, err := describeTargetGroup(ctx, clients.Config, req.Region, targetGroupName)
		if err != nil {
			return TrafficRoutingInspection{}, err
		}
		targetHealth, err := describeTargetHealth(ctx, clients.Config, req.Region, targetGroup.TargetGroupARN)
		if err != nil {
			return TrafficRoutingInspection{}, err
		}
		return trafficRoutingInspectionFromTargetGroup(req, targetGroup, targetHealth), nil
	default:
		return TrafficRoutingInspection{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonUnsupportedResourceType)
	}
}

func trafficRoutingInspectionFromApplication(req TrafficRoutingRequest, application codeDeployApplicationInfoPayload) TrafficRoutingInspection {
	deploymentGroups := append([]string(nil), codeDeployDeploymentGroups(application)...)
	sort.Strings(deploymentGroups)
	return TrafficRoutingInspection{
		Kind:           req.ResourceType,
		DisplayName:    trafficRoutingDisplayName(req.ResourceType),
		InspectionMode: "live",
		Summary:        trafficRoutingSummaryText(req.ResourceType, req.ResourceDescription),
		CodeDeployApplication: &CodeDeployApplicationInspection{
			ApplicationName:  application.ApplicationName,
			ApplicationID:    application.ApplicationID,
			ComputePlatform:  application.ComputePlatform,
			LinkedToGitHub:   application.LinkedToGitHub,
			DeploymentGroups: deploymentGroups,
		},
	}
}

func trafficRoutingInspectionFromDeploymentGroup(req TrafficRoutingRequest, group codeDeployDeploymentGroupInfoPayload) TrafficRoutingInspection {
	ecsServices := append([]CodeDeployECSServiceBinding(nil), codeDeployECSServices(group)...)
	sort.SliceStable(ecsServices, func(i, j int) bool {
		if ecsServices[i].ClusterName == ecsServices[j].ClusterName {
			return ecsServices[i].ServiceName < ecsServices[j].ServiceName
		}
		return ecsServices[i].ClusterName < ecsServices[j].ClusterName
	})
	targetGroups := codeDeployTargetGroups(group)
	sort.Strings(targetGroups)
	return TrafficRoutingInspection{
		Kind:           req.ResourceType,
		DisplayName:    trafficRoutingDisplayName(req.ResourceType),
		InspectionMode: "live",
		Summary:        trafficRoutingSummaryText(req.ResourceType, req.ResourceDescription),
		CodeDeployDeploymentGroup: &CodeDeployDeploymentGroupInspection{
			ApplicationName:       group.ApplicationName,
			DeploymentGroupName:   group.DeploymentGroupName,
			DeploymentConfigName:  group.DeploymentConfigName,
			ServiceRoleARN:        group.ServiceRoleARN,
			DeploymentType:        group.DeploymentStyle.DeploymentType,
			DeploymentOption:      group.DeploymentStyle.DeploymentOption,
			ECSServices:           ecsServices,
			TargetGroups:          targetGroups,
			ProductionListenerARN: codeDeployProductionListenerARN(group),
			TestListenerARN:       codeDeployTestListenerARN(group),
		},
	}
}

func trafficRoutingInspectionFromTargetGroup(req TrafficRoutingRequest, targetGroup elbv2TargetGroupPayload, targetHealth []elbv2TargetHealthDescriptionPayload) TrafficRoutingInspection {
	loadBalancerARNs := append([]string(nil), targetGroup.LoadBalancerARNs...)
	sort.Strings(loadBalancerARNs)
	healthCounts := make(map[string]int)
	targets := make([]TargetHealthInspection, 0, len(targetHealth))
	for _, health := range targetHealth {
		state := strings.TrimSpace(health.TargetHealth.State)
		if state != "" {
			healthCounts[state]++
		}
		target := TargetHealthInspection{
			TargetID:    strings.TrimSpace(health.Target.ID),
			Port:        health.Target.Port,
			State:       state,
			Reason:      strings.TrimSpace(health.TargetHealth.Reason),
			Description: strings.TrimSpace(health.TargetHealth.Description),
		}
		if target.TargetID == "" && target.Port == 0 && target.State == "" && target.Reason == "" && target.Description == "" {
			continue
		}
		targets = append(targets, target)
	}
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].TargetID == targets[j].TargetID {
			return targets[i].Port < targets[j].Port
		}
		return targets[i].TargetID < targets[j].TargetID
	})
	return TrafficRoutingInspection{
		Kind:           req.ResourceType,
		DisplayName:    trafficRoutingDisplayName(req.ResourceType),
		InspectionMode: "live",
		Summary:        trafficRoutingSummaryText(req.ResourceType, req.ResourceDescription),
		TargetGroup: &TargetGroupInspection{
			TargetGroupName:            strings.TrimSpace(targetGroup.TargetGroupName),
			TargetGroupARN:             strings.TrimSpace(targetGroup.TargetGroupARN),
			Protocol:                   strings.TrimSpace(targetGroup.Protocol),
			Port:                       targetGroup.Port,
			TargetType:                 strings.TrimSpace(targetGroup.TargetType),
			LoadBalancerARNs:           loadBalancerARNs,
			HealthCheckProtocol:        strings.TrimSpace(targetGroup.HealthCheckProtocol),
			HealthCheckPort:            strings.TrimSpace(targetGroup.HealthCheckPort),
			HealthCheckPath:            strings.TrimSpace(targetGroup.HealthCheckPath),
			HealthCheckIntervalSeconds: targetGroup.HealthCheckIntervalSeconds,
			HealthCheckTimeoutSeconds:  targetGroup.HealthCheckTimeoutSeconds,
			HealthyThresholdCount:      targetGroup.HealthyThresholdCount,
			UnhealthyThresholdCount:    targetGroup.UnhealthyThresholdCount,
			TargetHealthCounts:         healthCounts,
			Targets:                    targets,
		},
	}
}

func trafficRoutingInspectionFromListener(req TrafficRoutingRequest, listener elbv2ListenerPayload) TrafficRoutingInspection {
	defaultActions := append([]ListenerActionInspection(nil), listenerActions(listener)...)
	sort.SliceStable(defaultActions, func(i, j int) bool {
		if defaultActions[i].Order == defaultActions[j].Order {
			return defaultActions[i].Type < defaultActions[j].Type
		}
		return defaultActions[i].Order < defaultActions[j].Order
	})
	certificates := append([]string(nil), listenerCertificates(listener)...)
	sort.Strings(certificates)
	return TrafficRoutingInspection{
		Kind:           req.ResourceType,
		DisplayName:    trafficRoutingDisplayName(req.ResourceType),
		InspectionMode: "live",
		Summary:        trafficRoutingSummaryText(req.ResourceType, req.ResourceDescription),
		Listener: &ListenerInspection{
			ListenerARN:     listener.ListenerARN,
			LoadBalancerARN: listener.LoadBalancerARN,
			Port:            listener.Port,
			Protocol:        listener.Protocol,
			SSLPolicy:       listener.SSLPolicy,
			Certificates:    certificates,
			DefaultActions:  defaultActions,
		},
	}
}

func codeDeployApplicationName(deploymentName, resourceName, resourceType string) string {
	if resourceType == "codeDeployApplication" && strings.TrimSpace(resourceName) != "" {
		return strings.TrimSpace(resourceName)
	}
	return strings.TrimSpace(deploymentName) + "-codedeploy-app"
}

func codeDeployDeploymentGroupName(req TrafficRoutingRequest) string {
	if req.ResourceType == "codeDeployDeploymentGroup" && strings.TrimSpace(req.ResourceName) != "" {
		return strings.TrimSpace(req.ResourceName)
	}
	return strings.TrimSpace(req.DeploymentName) + "-codedeploy-group"
}

func codeDeployECSServices(group codeDeployDeploymentGroupInfoPayload) []CodeDeployECSServiceBinding {
	services := make([]CodeDeployECSServiceBinding, 0, len(group.ECSServices))
	for _, service := range group.ECSServices {
		if strings.TrimSpace(service.ClusterName) == "" && strings.TrimSpace(service.ServiceName) == "" {
			continue
		}
		services = append(services, CodeDeployECSServiceBinding{
			ClusterName: strings.TrimSpace(service.ClusterName),
			ServiceName: strings.TrimSpace(service.ServiceName),
		})
	}
	return services
}

func codeDeployDeploymentGroups(application codeDeployApplicationInfoPayload) []string {
	deploymentGroups := make([]string, 0, len(application.DeploymentGroups))
	for _, group := range application.DeploymentGroups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		deploymentGroups = append(deploymentGroups, group)
	}
	return deploymentGroups
}

func codeDeployTaskSetTargetGroupName(group codeDeployDeploymentGroupInfoPayload, resourceType, resourceName string) string {
	targetGroups := codeDeployTargetGroups(group)
	trimmedName := strings.TrimSpace(resourceName)
	if trimmedName != "" {
		for _, targetGroup := range targetGroups {
			if targetGroup == trimmedName {
				return targetGroup
			}
		}
	}
	switch resourceType {
	case "blueTaskSetTargetGroup":
		if len(targetGroups) > 0 {
			return targetGroups[0]
		}
	case "greenTaskSetTargetGroup":
		if len(targetGroups) > 1 {
			return targetGroups[1]
		}
		if len(targetGroups) == 1 {
			return targetGroups[0]
		}
	}
	return ""
}

func codeDeployTargetGroups(group codeDeployDeploymentGroupInfoPayload) []string {
	seen := make(map[string]struct{})
	targetGroups := make([]string, 0)
	for _, pair := range group.LoadBalancerInfo.TargetGroupPairInfoList {
		for _, target := range pair.TargetGroups {
			name := strings.TrimSpace(target.Name)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			targetGroups = append(targetGroups, name)
		}
	}
	return targetGroups
}

func codeDeployProductionListenerARN(group codeDeployDeploymentGroupInfoPayload) string {
	for _, pair := range group.LoadBalancerInfo.TargetGroupPairInfoList {
		for _, listenerARN := range pair.ProdTrafficRoute.ListenerARNs {
			listenerARN = strings.TrimSpace(listenerARN)
			if listenerARN != "" {
				return listenerARN
			}
		}
	}
	return ""
}

func codeDeployTestListenerARN(group codeDeployDeploymentGroupInfoPayload) string {
	for _, pair := range group.LoadBalancerInfo.TargetGroupPairInfoList {
		for _, listenerARN := range pair.TestTrafficRoute.ListenerARNs {
			listenerARN = strings.TrimSpace(listenerARN)
			if listenerARN != "" {
				return listenerARN
			}
		}
	}
	return ""
}

func listenerCertificates(listener elbv2ListenerPayload) []string {
	certificates := make([]string, 0, len(listener.Certificates))
	for _, certificate := range listener.Certificates {
		arn := strings.TrimSpace(certificate.CertificateARN)
		if arn == "" {
			continue
		}
		certificates = append(certificates, arn)
	}
	return certificates
}

func listenerActions(listener elbv2ListenerPayload) []ListenerActionInspection {
	actions := make([]ListenerActionInspection, 0, len(listener.DefaultActions))
	for _, action := range listener.DefaultActions {
		targetGroups := make([]ListenerActionTargetInspection, 0, len(action.ForwardConfig.TargetGroups))
		for _, targetGroup := range action.ForwardConfig.TargetGroups {
			targetGroups = append(targetGroups, ListenerActionTargetInspection{
				TargetGroupARN: strings.TrimSpace(targetGroup.TargetGroupARN),
				Weight:         targetGroup.Weight,
			})
		}
		sort.SliceStable(targetGroups, func(i, j int) bool {
			if targetGroups[i].Weight == targetGroups[j].Weight {
				return targetGroups[i].TargetGroupARN < targetGroups[j].TargetGroupARN
			}
			return targetGroups[i].Weight < targetGroups[j].Weight
		})
		actions = append(actions, ListenerActionInspection{
			Type:           strings.TrimSpace(action.Type),
			Order:          action.Order,
			TargetGroupARN: strings.TrimSpace(action.TargetGroupARN),
			TargetGroups:   targetGroups,
		})
	}
	return actions
}

func getCodeDeployApplication(ctx context.Context, cfg aws.Config, region, applicationName string) (codeDeployApplicationInfoPayload, error) {
	var response codeDeployGetApplicationOutput
	if err := doSignedAWSJSONRequest(
		ctx,
		cfg,
		"codedeploy",
		region,
		"https://codedeploy."+strings.TrimSpace(region)+".amazonaws.com/",
		"CodeDeploy_20141006.GetApplication",
		map[string]string{
			"applicationName": strings.TrimSpace(applicationName),
		},
		&response,
		func(headers http.Header, body []byte) error {
			code, _ := parseAWSJSONError(headers, body)
			switch code {
			case "ApplicationDoesNotExistException":
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
			default:
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)
			}
		},
	); err != nil {
		return codeDeployApplicationInfoPayload{}, err
	}
	if strings.TrimSpace(response.Application.ApplicationName) == "" {
		return codeDeployApplicationInfoPayload{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
	}
	return response.Application, nil
}

func getCodeDeployDeploymentGroup(ctx context.Context, cfg aws.Config, region, applicationName, deploymentGroupName string) (codeDeployDeploymentGroupInfoPayload, error) {
	var response codeDeployGetDeploymentGroupOutput
	if err := doSignedAWSJSONRequest(
		ctx,
		cfg,
		"codedeploy",
		region,
		"https://codedeploy."+strings.TrimSpace(region)+".amazonaws.com/",
		"CodeDeploy_20141006.GetDeploymentGroup",
		map[string]string{
			"applicationName":     strings.TrimSpace(applicationName),
			"deploymentGroupName": strings.TrimSpace(deploymentGroupName),
		},
		&response,
		func(headers http.Header, body []byte) error {
			code, _ := parseAWSJSONError(headers, body)
			switch code {
			case "ApplicationDoesNotExistException", "DeploymentGroupDoesNotExistException":
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
			default:
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)
			}
		},
	); err != nil {
		return codeDeployDeploymentGroupInfoPayload{}, err
	}
	if strings.TrimSpace(response.DeploymentGroupInfo.DeploymentGroupName) == "" {
		return codeDeployDeploymentGroupInfoPayload{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
	}
	return response.DeploymentGroupInfo, nil
}

func describeTargetGroup(ctx context.Context, cfg aws.Config, region, targetGroupName string) (elbv2TargetGroupPayload, error) {
	body, err := doSignedAWSQueryRequest(
		ctx,
		cfg,
		"elasticloadbalancing",
		region,
		"https://elasticloadbalancing."+strings.TrimSpace(region)+".amazonaws.com/",
		url.Values{
			"Action":         []string{"DescribeTargetGroups"},
			"Version":        []string{"2015-12-01"},
			"Names.member.1": []string{strings.TrimSpace(targetGroupName)},
		},
		func(responseBody []byte) error {
			code := parseAWSQueryError(responseBody)
			switch code {
			case "TargetGroupNotFound", "LoadBalancerNotFound":
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
			default:
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)
			}
		},
	)
	if err != nil {
		return elbv2TargetGroupPayload{}, err
	}
	var response elbv2DescribeTargetGroupsResponse
	if err := xml.Unmarshal(body, &response); err != nil {
		return elbv2TargetGroupPayload{}, err
	}
	if len(response.Result.TargetGroups) == 0 {
		return elbv2TargetGroupPayload{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
	}
	return response.Result.TargetGroups[0], nil
}

func describeTargetHealth(ctx context.Context, cfg aws.Config, region, targetGroupARN string) ([]elbv2TargetHealthDescriptionPayload, error) {
	body, err := doSignedAWSQueryRequest(
		ctx,
		cfg,
		"elasticloadbalancing",
		region,
		"https://elasticloadbalancing."+strings.TrimSpace(region)+".amazonaws.com/",
		url.Values{
			"Action":         []string{"DescribeTargetHealth"},
			"Version":        []string{"2015-12-01"},
			"TargetGroupArn": []string{strings.TrimSpace(targetGroupARN)},
		},
		func(responseBody []byte) error {
			code := parseAWSQueryError(responseBody)
			switch code {
			case "TargetGroupNotFound", "LoadBalancerNotFound":
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
			default:
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)
			}
		},
	)
	if err != nil {
		return nil, err
	}
	var response elbv2DescribeTargetHealthResponse
	if err := xml.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	return response.Result.TargetHealthDescriptions, nil
}

func describeListener(ctx context.Context, cfg aws.Config, region, listenerARN string) (elbv2ListenerPayload, error) {
	body, err := doSignedAWSQueryRequest(
		ctx,
		cfg,
		"elasticloadbalancing",
		region,
		"https://elasticloadbalancing."+strings.TrimSpace(region)+".amazonaws.com/",
		url.Values{
			"Action":                []string{"DescribeListeners"},
			"Version":               []string{"2015-12-01"},
			"ListenerArns.member.1": []string{strings.TrimSpace(listenerARN)},
		},
		func(responseBody []byte) error {
			code := parseAWSQueryError(responseBody)
			switch code {
			case "ListenerNotFound", "LoadBalancerNotFound":
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
			default:
				return resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)
			}
		},
	)
	if err != nil {
		return elbv2ListenerPayload{}, err
	}
	var response elbv2DescribeListenersResponse
	if err := xml.Unmarshal(body, &response); err != nil {
		return elbv2ListenerPayload{}, err
	}
	if len(response.Result.Listeners) == 0 {
		return elbv2ListenerPayload{}, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonResourceNotFound)
	}
	return response.Result.Listeners[0], nil
}

func doSignedAWSJSONRequest(
	ctx context.Context,
	cfg aws.Config,
	service string,
	region string,
	endpoint string,
	target string,
	payload any,
	out any,
	mapError func(http.Header, []byte) error,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-amz-json-1.1")
	request.Header.Set("X-Amz-Target", target)
	responseBody, _, err := doSignedAWSRequest(ctx, cfg, service, region, request, body, mapError)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return err
	}
	return nil
}

func doSignedAWSQueryRequest(
	ctx context.Context,
	cfg aws.Config,
	service string,
	region string,
	endpoint string,
	values url.Values,
	mapError func([]byte) error,
) ([]byte, error) {
	body := []byte(values.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	responseBody, _, err := doSignedAWSRequest(ctx, cfg, service, region, request, body, func(_ http.Header, responseBody []byte) error {
		return mapError(responseBody)
	})
	if err != nil {
		return nil, err
	}
	return responseBody, nil
}

func doSignedAWSRequest(
	ctx context.Context,
	cfg aws.Config,
	service string,
	region string,
	request *http.Request,
	body []byte,
	mapError func(http.Header, []byte) error,
) ([]byte, http.Header, error) {
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
		if mapError != nil {
			return nil, response.Header, mapError(response.Header, responseBody)
		}
		return nil, response.Header, resourceaws.NewUnavailableError(resourceaws.UnavailableReasonAWSUnavailable)
	}
	return responseBody, response.Header, nil
}

func parseAWSJSONError(headers http.Header, body []byte) (string, string) {
	type payload struct {
		Type    string `json:"__type"`
		Code    string `json:"code"`
		Message string `json:"message"`
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
	return strings.TrimSpace(code), strings.TrimSpace(errPayload.Message)
}

func parseAWSQueryError(body []byte) string {
	type response struct {
		Error struct {
			Code string `xml:"Code"`
		} `xml:"Error"`
	}
	var errResponse response
	_ = xml.Unmarshal(body, &errResponse)
	return strings.TrimSpace(errResponse.Error.Code)
}

type codeDeployGetApplicationOutput struct {
	Application codeDeployApplicationInfoPayload `json:"application"`
}

type codeDeployApplicationInfoPayload struct {
	ApplicationID    string   `json:"applicationId"`
	ApplicationName  string   `json:"applicationName"`
	ComputePlatform  string   `json:"computePlatform"`
	LinkedToGitHub   bool     `json:"linkedToGitHub"`
	DeploymentGroups []string `json:"deploymentGroups"`
}

type codeDeployGetDeploymentGroupOutput struct {
	DeploymentGroupInfo codeDeployDeploymentGroupInfoPayload `json:"deploymentGroupInfo"`
}

type codeDeployDeploymentGroupInfoPayload struct {
	ApplicationName      string                            `json:"applicationName"`
	DeploymentGroupName  string                            `json:"deploymentGroupName"`
	DeploymentConfigName string                            `json:"deploymentConfigName"`
	ServiceRoleARN       string                            `json:"serviceRoleArn"`
	DeploymentStyle      codeDeployDeploymentStylePayload  `json:"deploymentStyle"`
	ECSServices          []codeDeployECSServicePayload     `json:"ecsServices"`
	LoadBalancerInfo     codeDeployLoadBalancerInfoPayload `json:"loadBalancerInfo"`
}

type codeDeployDeploymentStylePayload struct {
	DeploymentType   string `json:"deploymentType"`
	DeploymentOption string `json:"deploymentOption"`
}

type codeDeployECSServicePayload struct {
	ClusterName string `json:"clusterName"`
	ServiceName string `json:"serviceName"`
}

type codeDeployLoadBalancerInfoPayload struct {
	TargetGroupPairInfoList []codeDeployTargetGroupPairInfoPayload `json:"targetGroupPairInfoList"`
}

type codeDeployTargetGroupPairInfoPayload struct {
	ProdTrafficRoute codeDeployTrafficRoutePayload  `json:"prodTrafficRoute"`
	TestTrafficRoute codeDeployTrafficRoutePayload  `json:"testTrafficRoute"`
	TargetGroups     []codeDeployTargetGroupPayload `json:"targetGroups"`
}

type codeDeployTrafficRoutePayload struct {
	ListenerARNs []string `json:"listenerArns"`
}

type codeDeployTargetGroupPayload struct {
	Name string `json:"name"`
}

type elbv2DescribeTargetGroupsResponse struct {
	Result elbv2DescribeTargetGroupsResult `xml:"DescribeTargetGroupsResult"`
}

type elbv2DescribeTargetGroupsResult struct {
	TargetGroups []elbv2TargetGroupPayload `xml:"TargetGroups>member"`
}

type elbv2TargetGroupPayload struct {
	TargetGroupARN             string   `xml:"TargetGroupArn"`
	TargetGroupName            string   `xml:"TargetGroupName"`
	Protocol                   string   `xml:"Protocol"`
	Port                       int32    `xml:"Port"`
	TargetType                 string   `xml:"TargetType"`
	LoadBalancerARNs           []string `xml:"LoadBalancerArns>member"`
	HealthCheckProtocol        string   `xml:"HealthCheckProtocol"`
	HealthCheckPort            string   `xml:"HealthCheckPort"`
	HealthCheckPath            string   `xml:"HealthCheckPath"`
	HealthCheckIntervalSeconds int32    `xml:"HealthCheckIntervalSeconds"`
	HealthCheckTimeoutSeconds  int32    `xml:"HealthCheckTimeoutSeconds"`
	HealthyThresholdCount      int32    `xml:"HealthyThresholdCount"`
	UnhealthyThresholdCount    int32    `xml:"UnhealthyThresholdCount"`
}

type elbv2DescribeTargetHealthResponse struct {
	Result elbv2DescribeTargetHealthResult `xml:"DescribeTargetHealthResult"`
}

type elbv2DescribeTargetHealthResult struct {
	TargetHealthDescriptions []elbv2TargetHealthDescriptionPayload `xml:"TargetHealthDescriptions>member"`
}

type elbv2TargetHealthDescriptionPayload struct {
	Target       elbv2TargetDescriptionPayload `xml:"Target"`
	TargetHealth elbv2TargetHealthPayload      `xml:"TargetHealth"`
}

type elbv2TargetDescriptionPayload struct {
	ID   string `xml:"Id"`
	Port int32  `xml:"Port"`
}

type elbv2TargetHealthPayload struct {
	State       string `xml:"State"`
	Reason      string `xml:"Reason"`
	Description string `xml:"Description"`
}

type elbv2DescribeListenersResponse struct {
	Result elbv2DescribeListenersResult `xml:"DescribeListenersResult"`
}

type elbv2DescribeListenersResult struct {
	Listeners []elbv2ListenerPayload `xml:"Listeners>member"`
}

type elbv2ListenerPayload struct {
	ListenerARN     string                    `xml:"ListenerArn"`
	LoadBalancerARN string                    `xml:"LoadBalancerArn"`
	Port            int32                     `xml:"Port"`
	Protocol        string                    `xml:"Protocol"`
	SSLPolicy       string                    `xml:"SslPolicy"`
	Certificates    []elbv2CertificatePayload `xml:"Certificates>member"`
	DefaultActions  []elbv2ActionPayload      `xml:"DefaultActions>member"`
}

type elbv2CertificatePayload struct {
	CertificateARN string `xml:"CertificateArn"`
}

type elbv2ActionPayload struct {
	Type           string                    `xml:"Type"`
	Order          int32                     `xml:"Order"`
	TargetGroupARN string                    `xml:"TargetGroupArn"`
	ForwardConfig  elbv2ForwardConfigPayload `xml:"ForwardConfig"`
}

type elbv2ForwardConfigPayload struct {
	TargetGroups []elbv2TargetGroupTuplePayload `xml:"TargetGroups>member"`
}

type elbv2TargetGroupTuplePayload struct {
	TargetGroupARN string `xml:"TargetGroupArn"`
	Weight         int32  `xml:"Weight"`
}
