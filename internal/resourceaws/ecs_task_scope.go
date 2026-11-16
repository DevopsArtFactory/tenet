package resourceaws

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

var ErrECSTaskScope = errors.New("ECS task is outside the deployment scope")

// ECSTaskScope binds a selected task to the deployment's trusted service target.
type ECSTaskScope struct {
	AccountID       string
	Region          string
	ClusterARN      string
	ServiceARN      string
	TaskARN         string
	ObserverRoleARN string
}

func (s ECSTaskScope) Validate() error {
	cluster, err := arn.Parse(s.ClusterARN)
	if err != nil || s.AccountID == "" || s.Region == "" || cluster.Service != "ecs" || cluster.AccountID != s.AccountID || cluster.Region != s.Region || cluster.Partition == "" {
		return ErrECSTaskScope
	}
	clusterParts := strings.Split(cluster.Resource, "/")
	if len(clusterParts) != 2 || clusterParts[0] != "cluster" || clusterParts[1] == "" {
		return ErrECSTaskScope
	}
	for _, target := range []struct{ value, kind string }{{s.ServiceARN, "service"}, {s.TaskARN, "task"}} {
		parsed, err := arn.Parse(target.value)
		if err != nil || parsed.Partition != cluster.Partition || parsed.Service != "ecs" || parsed.AccountID != s.AccountID || parsed.Region != s.Region {
			return ErrECSTaskScope
		}
		parts := strings.Split(parsed.Resource, "/")
		if len(parts) != 3 || parts[0] != target.kind || parts[1] != clusterParts[1] || parts[2] == "" {
			return ErrECSTaskScope
		}
	}
	return nil
}

// AuthorizeECSTask checks live membership before either inspection or log reads.
func (c *ClientCache) AuthorizeECSTask(ctx context.Context, scope ECSTaskScope) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	clients, err := c.Clients(ctx, scope.Region, scope.AccountID, scope.ObserverRoleARN)
	if err != nil {
		return err
	}
	out, err := clients.ECS.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: aws.String(scope.ClusterARN), Tasks: []string{scope.TaskARN}})
	if err != nil {
		return err
	}
	if out == nil || len(out.Failures) != 0 || len(out.Tasks) != 1 {
		return ErrECSTaskScope
	}
	task := out.Tasks[0]
	serviceName := scope.ServiceARN[strings.LastIndex(scope.ServiceARN, "/")+1:]
	if aws.ToString(task.TaskArn) != scope.TaskARN || aws.ToString(task.ClusterArn) != scope.ClusterARN || aws.ToString(task.Group) != "service:"+serviceName {
		return ErrECSTaskScope
	}
	return nil
}
