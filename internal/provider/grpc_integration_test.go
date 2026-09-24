package provider

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/o11y-one/terraform-provider-o11y/internal/client"
	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
	"github.com/o11y-one/terraform-provider-o11y/internal/idempotency"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type alertTestServer struct {
	alertsv1.UnimplementedAlertDefinitionServiceServer
	alertsv1.UnimplementedAlertRuntimeServiceServer
	alertsv1.UnimplementedAlertNotificationServiceServer
	alertsv1.UnimplementedAlertPreviewServiceServer
	alertsv1.UnimplementedAlertSloServiceServer
	mu           sync.Mutex
	destinations map[string]*alertsv1.AlertDestinationV1
	policies     map[string]*alertsv1.AlertNotificationPolicyV1
	definitions  map[string]*alertsv1.AlertDefinitionV1
	windows      map[string]*alertsv1.AlertMaintenanceWindowV1
	silences     map[string]*alertsv1.AlertSilenceV1
	slis         map[string]*alertsv1.AlertSliV1
	slos         map[string]*alertsv1.AlertSloV1
	templates    map[string]*alertsv1.AlertNotificationTemplateV1
	idempotent   map[string]string
	next         int
}

const testTenantID = "019f430f-90d4-74c3-95b7-9120db366252"
const testOrgID = "019f430f-90d4-74c3-95b7-9120db366253"

func newAlertTestServer() *alertTestServer {
	return &alertTestServer{destinations: map[string]*alertsv1.AlertDestinationV1{}, policies: map[string]*alertsv1.AlertNotificationPolicyV1{}, definitions: map[string]*alertsv1.AlertDefinitionV1{}, windows: map[string]*alertsv1.AlertMaintenanceWindowV1{}, silences: map[string]*alertsv1.AlertSilenceV1{}, slis: map[string]*alertsv1.AlertSliV1{}, slos: map[string]*alertsv1.AlertSloV1{}, templates: map[string]*alertsv1.AlertNotificationTemplateV1{}, idempotent: map[string]string{}}
}
func (s *alertTestServer) authorize(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	if first(md.Get("x-o11y-key")) != "test-token" || len(md.Get("authorization")) != 0 || first(md.Get("x-o11y-tenant-id")) != testTenantID || first(md.Get("x-o11y-org-id")) != testOrgID || first(md.Get("x-o11y-managed-by")) != "terraform" {
		return status.Error(codes.PermissionDenied, "invalid provider scope")
	}
	return nil
}
func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
func (s *alertTestServer) id(prefix, key string) string {
	if id := s.idempotent[key]; id != "" {
		return id
	}
	s.next++
	id := prefix + "-" + string(rune('0'+s.next))
	s.idempotent[key] = id
	return id
}

func (s *alertTestServer) CreateSlo(ctx context.Context, req *alertsv1.CreateSloRequest) (*alertsv1.AlertSloV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id("slo", req.IdempotencyKey)
	if existing := s.slos[id]; existing != nil {
		return proto.Clone(existing).(*alertsv1.AlertSloV1), nil
	}
	item := testSLO(id, req.SloKey, req.Name, req.Description, req.Revision, 1)
	s.slos[id] = item
	return proto.Clone(item).(*alertsv1.AlertSloV1), nil
}

func (s *alertTestServer) CreateSli(ctx context.Context, req *alertsv1.CreateSliRequest) (*alertsv1.AlertSliV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id("sli", req.IdempotencyKey)
	if existing := s.slis[id]; existing != nil {
		return proto.Clone(existing).(*alertsv1.AlertSliV1), nil
	}
	item := testSLI(id, req.SliKey, req.Name, req.Description, req.Revision, 1)
	s.slis[id] = item
	return proto.Clone(item).(*alertsv1.AlertSliV1), nil
}

func (s *alertTestServer) UpdateSli(ctx context.Context, req *alertsv1.UpdateSliRequest) (*alertsv1.AlertSliV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.slis[req.SliId]
	if existing == nil {
		return nil, status.Error(codes.NotFound, "sli")
	}
	if existing.CurrentRevisionId != req.ExpectedRevisionId {
		return nil, status.Error(codes.Aborted, "stale SLI revision")
	}
	item := testSLI(existing.Id, existing.SliKey, req.Name, req.Description, req.Revision, existing.CurrentRevision.RevisionNumber+1)
	s.slis[req.SliId] = item
	return proto.Clone(item).(*alertsv1.AlertSliV1), nil
}

func (s *alertTestServer) GetSli(ctx context.Context, req *alertsv1.GetSliRequest) (*alertsv1.AlertSliV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.slis[req.SliId]
	if item == nil {
		return nil, status.Error(codes.NotFound, "sli")
	}
	return proto.Clone(item).(*alertsv1.AlertSliV1), nil
}

func (s *alertTestServer) ArchiveSli(ctx context.Context, req *alertsv1.ArchiveSliRequest) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slis[req.SliId] == nil {
		return nil, status.Error(codes.NotFound, "sli")
	}
	delete(s.slis, req.SliId)
	return &alertsv1.AlertMutationResponse{Ok: true, ResourceId: req.SliId}, nil
}

func testSLI(id, key, name, description string, input *alertsv1.SliRevisionInputV1, revisionNumber int32) *alertsv1.AlertSliV1 {
	now := timestamppb.New(time.Date(2030, 1, 1, 0, int(revisionNumber), 0, 0, time.UTC))
	revisionID := fmt.Sprintf("%s-revision-%d", id, revisionNumber)
	revision := &alertsv1.AlertSliRevisionV1{Id: revisionID, SliId: id, RevisionNumber: revisionNumber, IndicatorKind: input.IndicatorKind, Scope: input.Scope, Owner: input.Owner, EligibleEvents: input.EligibleEvents, GoodEvents: input.GoodEvents, ExcludedEvents: input.ExcludedEvents, Aggregation: input.Aggregation, MissingDataBehavior: input.MissingDataBehavior, ConfigHash: fmt.Sprintf("config-%d", revisionNumber), CreatedAt: now, LatencyThreshold: input.LatencyThreshold}
	return &alertsv1.AlertSliV1{Id: id, SliKey: key, Name: name, Description: description, CurrentRevisionId: revisionID, Provenance: "terraform", CreatedAt: now, UpdatedAt: now, CurrentRevision: revision}
}

func (s *alertTestServer) UpdateSlo(ctx context.Context, req *alertsv1.UpdateSloRequest) (*alertsv1.AlertSloV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.slos[req.SloId]
	if existing == nil {
		return nil, status.Error(codes.NotFound, "slo")
	}
	if req.ExpectedRevisionId != existing.CurrentRevisionId {
		return nil, status.Error(codes.Aborted, "stale SLO revision")
	}
	if id := s.idempotent[req.IdempotencyKey]; id != "" {
		return proto.Clone(s.slos[id]).(*alertsv1.AlertSloV1), nil
	}
	s.idempotent[req.IdempotencyKey] = req.SloId
	item := testSLO(req.SloId, existing.SloKey, req.Name, req.Description, req.Revision, existing.CurrentRevision.RevisionNumber+1)
	s.slos[req.SloId] = item
	return proto.Clone(item).(*alertsv1.AlertSloV1), nil
}

func (s *alertTestServer) GetSlo(ctx context.Context, req *alertsv1.GetSloRequest) (*alertsv1.AlertSloV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.slos[req.SloId]
	if item == nil {
		return nil, status.Error(codes.NotFound, "slo")
	}
	return proto.Clone(item).(*alertsv1.AlertSloV1), nil
}

func (s *alertTestServer) ArchiveSlo(ctx context.Context, req *alertsv1.ArchiveSloRequest) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slos[req.SloId] == nil {
		return nil, status.Error(codes.NotFound, "slo")
	}
	delete(s.slos, req.SloId)
	return &alertsv1.AlertMutationResponse{Ok: true, ResourceId: req.SloId}, nil
}

func testSLO(id, key, name, description string, input *alertsv1.SloRevisionInputV1, revisionNumber int32) *alertsv1.AlertSloV1 {
	now := timestamppb.New(time.Date(2030, 1, 1, 0, int(revisionNumber), 0, 0, time.UTC))
	maximumWindow := input.RollingWindowSeconds
	if input.WindowMode == alertsv1.SloWindowModeV1_SLO_WINDOW_MODE_V1_CALENDAR {
		maximumWindow = maximumSLOWindowSeconds
	}
	revisionID := fmt.Sprintf("%s-revision-%d", id, revisionNumber)
	revision := &alertsv1.AlertSloRevisionV1{
		Id: revisionID, SloId: id, RevisionNumber: revisionNumber, SliId: input.SliId,
		SliRevisionId: input.SliRevisionId, TargetRatio: input.TargetRatio,
		RollingWindowSeconds: input.RollingWindowSeconds, Owner: input.Owner, Labels: input.Labels,
		ConfigHash: fmt.Sprintf("config-%d", revisionNumber), CreatedAt: now,
		WindowMode: input.WindowMode, CalendarPeriod: input.CalendarPeriod,
		CalendarTimezone: input.CalendarTimezone, EffectiveFrom: now,
		MaximumWindowSeconds: maximumWindow,
	}
	return &alertsv1.AlertSloV1{
		Id: id, SloKey: key, Name: name, Description: description,
		CurrentRevisionId: revisionID, Provenance: "terraform", CreatedAt: now,
		UpdatedAt: now, CurrentRevision: revision,
	}
}

func (s *alertTestServer) CreateDestination(ctx context.Context, req *alertsv1.UpsertDestinationRequest) (*alertsv1.AlertDestinationV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id("destination", req.IdempotencyKey)
	item := &alertsv1.AlertDestinationV1{Id: id, DestinationKey: req.DestinationKey, Name: req.Name, Kind: req.Kind, Enabled: req.Enabled, Config: req.Config, SecretRefs: req.SecretRefs}
	s.destinations[id] = item
	return item, nil
}
func (s *alertTestServer) UpdateDestination(ctx context.Context, req *alertsv1.UpsertDestinationRequest) (*alertsv1.AlertDestinationV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destinations[req.Id] == nil {
		return nil, status.Error(codes.NotFound, "destination")
	}
	item := &alertsv1.AlertDestinationV1{Id: req.Id, DestinationKey: req.DestinationKey, Name: req.Name, Kind: req.Kind, Enabled: req.Enabled, Config: req.Config, SecretRefs: req.SecretRefs}
	s.destinations[req.Id] = item
	return item, nil
}
func (s *alertTestServer) ListDestinations(ctx context.Context, req *alertsv1.ListDestinationsRequest) (*alertsv1.ListDestinationsResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]*alertsv1.AlertDestinationV1, 0, len(s.destinations))
	for _, v := range s.destinations {
		items = append(items, v)
	}
	start, end := pageBounds(len(items), req.Offset, req.Limit)
	out := &alertsv1.ListDestinationsResponse{Items: items[start:end]}
	return out, nil
}
func (s *alertTestServer) GetDestination(ctx context.Context, req *alertsv1.GetAlertResourceRequest) (*alertsv1.AlertDestinationV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.destinations[req.Id]
	if item == nil {
		return nil, status.Error(codes.NotFound, "destination")
	}
	return item, nil
}
func (s *alertTestServer) DeleteDestination(ctx context.Context, req *alertsv1.DeleteDestinationRequest) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.destinations, req.Id)
	return &alertsv1.AlertMutationResponse{Ok: true, ResourceId: req.Id}, nil
}
func (s *alertTestServer) CreateNotificationPolicy(ctx context.Context, req *alertsv1.UpsertNotificationPolicyRequest) (*alertsv1.AlertNotificationPolicyV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id("policy", req.IdempotencyKey)
	item := &alertsv1.AlertNotificationPolicyV1{Id: id, PolicyKey: req.PolicyKey, Name: req.Name, Enabled: req.Enabled, Config: expandedTestNotificationPolicyConfig(req.Config), Revision: 1}
	s.policies[id] = item
	return item, nil
}
func (s *alertTestServer) UpdateNotificationPolicy(ctx context.Context, req *alertsv1.UpsertNotificationPolicyRequest) (*alertsv1.AlertNotificationPolicyV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.policies[req.Id]
	if existing == nil {
		return nil, status.Error(codes.NotFound, "policy")
	}
	if req.ExpectedRevision == nil || req.GetExpectedRevision() != existing.Revision {
		return nil, status.Error(codes.Aborted, "stale policy revision")
	}
	item := &alertsv1.AlertNotificationPolicyV1{Id: req.Id, PolicyKey: req.PolicyKey, Name: req.Name, Enabled: req.Enabled, Config: expandedTestNotificationPolicyConfig(req.Config), Revision: existing.Revision + 1}
	s.policies[req.Id] = item
	return item, nil
}

func expandedTestNotificationPolicyConfig(input *alertsv1.AlertNotificationPolicyConfigV1) *alertsv1.AlertNotificationPolicyConfigV1 {
	config := proto.Clone(input).(*alertsv1.AlertNotificationPolicyConfigV1)
	if config.Tree == nil {
		return config
	}
	for _, node := range config.Tree.Nodes {
		if node.NodeKind != alertsv1.AlertPolicyTreeNodeKindV1_ALERT_POLICY_TREE_NODE_KIND_V1_ROUTE {
			continue
		}
		config.Routes = append(config.Routes, &alertsv1.AlertNotificationRouteV1{
			Id:       "persisted-" + node.NodeKey,
			RouteKey: node.NodeKey,
			Priority: node.Priority,
			Enabled:  node.Enabled,
			Target:   node.Target,
			Behavior: node.Behavior,
			TreePath: []string{node.ParentNodeKey, node.NodeKey},
		})
	}
	return config
}
func (s *alertTestServer) ListNotificationPolicies(ctx context.Context, req *alertsv1.ListOperatorResourcesRequest) (*alertsv1.ListNotificationPoliciesResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]*alertsv1.AlertNotificationPolicyV1, 0, len(s.policies))
	for _, v := range s.policies {
		items = append(items, v)
	}
	start, end := pageBounds(len(items), req.Offset, req.Limit)
	out := &alertsv1.ListNotificationPoliciesResponse{Items: items[start:end]}
	return out, nil
}
func (s *alertTestServer) GetNotificationPolicy(ctx context.Context, req *alertsv1.GetAlertResourceRequest) (*alertsv1.AlertNotificationPolicyV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.policies[req.Id]
	if item == nil {
		return nil, status.Error(codes.NotFound, "policy")
	}
	return item, nil
}

func pageBounds(length int, offset, limit int32) (int, int) {
	start := int(offset)
	if start > length {
		start = length
	}
	if limit <= 0 {
		return start, length
	}
	end := start + int(limit)
	if end > length {
		end = length
	}
	return start, end
}
func (s *alertTestServer) DeleteNotificationPolicy(ctx context.Context, req *alertsv1.DeleteOperatorResourceRequest) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.policies, req.Id)
	return &alertsv1.AlertMutationResponse{Ok: true, ResourceId: req.Id}, nil
}

func (s *alertTestServer) CreateNotificationTemplate(ctx context.Context, req *alertsv1.UpsertAlertNotificationTemplateRequest) (*alertsv1.AlertNotificationTemplateV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id("template", req.IdempotencyKey)
	if existing := s.templates[id]; existing != nil {
		return proto.Clone(existing).(*alertsv1.AlertNotificationTemplateV1), nil
	}
	item := testNotificationTemplate(id, req, 1, "")
	s.templates[id] = item
	return proto.Clone(item).(*alertsv1.AlertNotificationTemplateV1), nil
}

func (s *alertTestServer) UpdateNotificationTemplate(ctx context.Context, req *alertsv1.UpsertAlertNotificationTemplateRequest) (*alertsv1.AlertNotificationTemplateV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.templates[req.Id]
	if existing == nil {
		return nil, status.Error(codes.NotFound, "notification template")
	}
	if req.ExpectedRevision == nil || req.GetExpectedRevision() != existing.Revision {
		return nil, status.Error(codes.Aborted, "stale notification template revision")
	}
	item := testNotificationTemplate(req.Id, req, existing.Revision+1, existing.PublishedRevisionId)
	item.ArchivedAt = existing.ArchivedAt
	s.templates[req.Id] = item
	return proto.Clone(item).(*alertsv1.AlertNotificationTemplateV1), nil
}

func (s *alertTestServer) PublishNotificationTemplate(ctx context.Context, req *alertsv1.PublishAlertNotificationTemplateRequest) (*alertsv1.AlertNotificationTemplateV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.templates[req.NotificationTemplateId]
	if item == nil {
		return nil, status.Error(codes.NotFound, "notification template")
	}
	if item.Revision != req.ExpectedRevision {
		return nil, status.Error(codes.Aborted, "stale notification template revision")
	}
	item.PublishedRevisionId = item.CurrentRevisionId
	item.Revision++
	item.CurrentRevision.HasBeenPublished = true
	item.CurrentRevision.FirstPublishedAt = timestamppb.Now()
	return proto.Clone(item).(*alertsv1.AlertNotificationTemplateV1), nil
}

func (s *alertTestServer) SetNotificationTemplateArchived(ctx context.Context, req *alertsv1.SetAlertNotificationTemplateArchivedRequest) (*alertsv1.AlertNotificationTemplateV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.templates[req.NotificationTemplateId]
	if item == nil {
		return nil, status.Error(codes.NotFound, "notification template")
	}
	if item.Revision != req.ExpectedRevision {
		return nil, status.Error(codes.Aborted, "stale notification template revision")
	}
	if req.Archived {
		item.ArchivedAt = timestamppb.Now()
	} else {
		item.ArchivedAt = nil
	}
	item.Revision++
	return proto.Clone(item).(*alertsv1.AlertNotificationTemplateV1), nil
}

func (s *alertTestServer) GetNotificationTemplate(ctx context.Context, req *alertsv1.GetAlertNotificationTemplateRequest) (*alertsv1.AlertNotificationTemplateV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.templates[req.NotificationTemplateId]
	if item == nil {
		return nil, status.Error(codes.NotFound, "notification template")
	}
	return proto.Clone(item).(*alertsv1.AlertNotificationTemplateV1), nil
}

func testNotificationTemplate(id string, req *alertsv1.UpsertAlertNotificationTemplateRequest, revision int64, publishedRevisionID string) *alertsv1.AlertNotificationTemplateV1 {
	now := timestamppb.New(time.Date(2030, 1, 1, 0, int(revision), 0, 0, time.UTC))
	revisionID := fmt.Sprintf("%s-revision-%d", id, revision)
	current := &alertsv1.AlertNotificationTemplateRevisionV1{Id: revisionID, NotificationTemplateId: id, RevisionNumber: revision, Document: req.Document, VariableSchemaVersion: 1, ContentHash: fmt.Sprintf("content-%d", revision), ChangeReason: req.ChangeReason, CreatedAt: now}
	return &alertsv1.AlertNotificationTemplateV1{Id: id, TemplateKey: req.TemplateKey, Name: req.Name, Description: req.Description, CurrentRevisionId: revisionID, PublishedRevisionId: publishedRevisionID, Revision: revision, Provenance: "terraform", ProvenanceRef: req.ProvenanceRef, CreatedAt: now, UpdatedAt: now, CurrentRevision: current, Kind: alertsv1.AlertNotificationTemplateKindV1_ALERT_NOTIFICATION_TEMPLATE_KIND_V1_USER}
}

func (s *alertTestServer) CreateMaintenanceWindow(ctx context.Context, req *alertsv1.UpsertMaintenanceWindowRequest) (*alertsv1.AlertMaintenanceWindowV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id("window", req.IdempotencyKey)
	item := &alertsv1.AlertMaintenanceWindowV1{Id: id, WindowKey: req.WindowKey, Name: req.Name, Scope: req.Scope, StartsAt: req.StartsAt, EndsAt: req.EndsAt}
	s.windows[id] = item
	return item, nil
}
func (s *alertTestServer) UpdateMaintenanceWindow(ctx context.Context, req *alertsv1.UpsertMaintenanceWindowRequest) (*alertsv1.AlertMaintenanceWindowV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.windows[req.Id] == nil {
		return nil, status.Error(codes.NotFound, "window")
	}
	item := &alertsv1.AlertMaintenanceWindowV1{Id: req.Id, WindowKey: req.WindowKey, Name: req.Name, Scope: req.Scope, StartsAt: req.StartsAt, EndsAt: req.EndsAt}
	s.windows[req.Id] = item
	return item, nil
}
func (s *alertTestServer) GetMaintenanceWindow(ctx context.Context, req *alertsv1.GetAlertResourceRequest) (*alertsv1.AlertMaintenanceWindowV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.windows[req.Id]
	if item == nil {
		return nil, status.Error(codes.NotFound, "window")
	}
	return item, nil
}
func (s *alertTestServer) DeleteMaintenanceWindow(ctx context.Context, req *alertsv1.DeleteOperatorResourceRequest) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.windows, req.Id)
	return &alertsv1.AlertMutationResponse{Ok: true, ResourceId: req.Id}, nil
}
func (s *alertTestServer) CreateSilence(ctx context.Context, req *alertsv1.UpsertSilenceRequest) (*alertsv1.AlertSilenceV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id("silence", req.IdempotencyKey)
	item := &alertsv1.AlertSilenceV1{Id: id, SilenceKey: req.SilenceKey, Matcher: req.Matcher, Reason: req.Reason, StartsAt: req.StartsAt, EndsAt: req.EndsAt}
	s.silences[id] = item
	return item, nil
}
func (s *alertTestServer) UpdateSilence(ctx context.Context, req *alertsv1.UpsertSilenceRequest) (*alertsv1.AlertSilenceV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.silences[req.Id] == nil {
		return nil, status.Error(codes.NotFound, "silence")
	}
	item := &alertsv1.AlertSilenceV1{Id: req.Id, SilenceKey: req.SilenceKey, Matcher: req.Matcher, Reason: req.Reason, StartsAt: req.StartsAt, EndsAt: req.EndsAt}
	s.silences[req.Id] = item
	return item, nil
}
func (s *alertTestServer) GetSilence(ctx context.Context, req *alertsv1.GetAlertResourceRequest) (*alertsv1.AlertSilenceV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.silences[req.Id]
	if item == nil {
		return nil, status.Error(codes.NotFound, "silence")
	}
	return item, nil
}
func (s *alertTestServer) DeleteSilence(ctx context.Context, req *alertsv1.DeleteOperatorResourceRequest) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.silences, req.Id)
	return &alertsv1.AlertMutationResponse{Ok: true, ResourceId: req.Id}, nil
}

func (s *alertTestServer) createAlert(ctx context.Context, base *alertsv1.AlertRecipeBaseV1, detectorKind string, detectorConfig *alertsv1.AlertDetectorConfigV1) (*alertsv1.CreateAlertDefinitionResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.id("alert", base.IdempotencyKey)
	owner := proto.Clone(base.Owner).(*alertsv1.AlertOwnerRefV1)
	owner.DisplayName = "Engineering"
	owner.Active = true
	// Like scope_json then scope_proto: an unset source is stored as fixed, and string values come back as typed_values too.
	scope := proto.Clone(base.Scope).(*alertsv1.AlertScopeV1)
	for _, filter := range scope.GetTelemetryAttributeFilters() {
		if filter.Source == alertsv1.SliTelemetryAttributeSourceV1_SLI_TELEMETRY_ATTRIBUTE_SOURCE_V1_UNSPECIFIED {
			filter.Source = alertsv1.SliTelemetryAttributeSourceV1_SLI_TELEMETRY_ATTRIBUTE_SOURCE_V1_FIXED
		}
		if len(filter.TypedValues) == 0 {
			for _, value := range filter.Values {
				filter.TypedValues = append(filter.TypedValues, &alertsv1.AlertScalarValueV1{Value: &alertsv1.AlertScalarValueV1_StringValue{StringValue: value}})
			}
		}
	}
	item := &alertsv1.AlertDefinitionV1{Id: id, TenantId: testTenantID, OrgId: base.OrgId, Slug: base.Slug, Name: base.Name, Description: base.Description, Severity: base.Severity, Mode: alertsv1.AlertModeV1_ALERT_MODE_V1_OBSERVE, Scope: scope, Owner: owner, Action: base.Action, EvaluationSettings: base.EvaluationSettings, SampleGuard: base.SampleGuard, CurrentRevisionId: "revision-1", DetectorKind: detectorKind, DetectorConfig: detectorConfig, EvaluationIntervalSeconds: base.EvaluationIntervalSeconds}
	s.definitions[id] = item
	return &alertsv1.CreateAlertDefinitionResponse{Definition: item, RevisionId: "revision-1"}, nil
}
func (s *alertTestServer) CreateAgentQualityRegressionAlert(ctx context.Context, req *alertsv1.CreateAgentQualityRegressionAlertRequest) (*alertsv1.CreateAlertDefinitionResponse, error) {
	return s.createAlert(ctx, req.Base, "agent_quality_regression", &alertsv1.AlertDetectorConfigV1{Config: &alertsv1.AlertDetectorConfigV1_AgentQualityRegression{AgentQualityRegression: req.RecipeConfig}})
}
func (s *alertTestServer) CreateCostPerSuccessAlert(ctx context.Context, req *alertsv1.CreateCostPerSuccessAlertRequest) (*alertsv1.CreateAlertDefinitionResponse, error) {
	return s.createAlert(ctx, req.Base, "cost_per_success_regression", &alertsv1.AlertDetectorConfigV1{Config: &alertsv1.AlertDetectorConfigV1_CostPerSuccess{CostPerSuccess: req.RecipeConfig}})
}

// Like create_slo_burn_alert: the ids are required, the current SLO revision overwrites the objective fields,
// and the SLO's SLI revision overwrites the scope.
func (s *alertTestServer) CreateSloBurnAlert(ctx context.Context, req *alertsv1.CreateSloBurnAlertRequest) (*alertsv1.CreateAlertDefinitionResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	config := proto.Clone(req.RecipeConfig).(*alertsv1.SloBurnConfigV1)
	if config.GetSloId() == "" || config.GetSloRevisionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "detector_config.slo_id and detector_config.slo_revision_id are required")
	}
	s.mu.Lock()
	slo := s.slos[config.SloId]
	sli := s.slis[slo.GetCurrentRevision().GetSliId()]
	s.mu.Unlock()
	if slo == nil || sli == nil {
		return nil, status.Error(codes.FailedPrecondition, "referenced SLO or its SLI revision is unavailable")
	}
	if slo.CurrentRevisionId != config.SloRevisionId {
		return nil, status.Error(codes.FailedPrecondition, "SLO burn alerts must reference the current SLO revision")
	}
	revision := slo.CurrentRevision
	config.SloWindowSeconds = proto.Int64(revision.MaximumWindowSeconds)
	config.TargetPercent = proto.Float64(revision.TargetRatio * 100)
	config.WindowMode = revision.WindowMode
	config.CalendarPeriod = revision.CalendarPeriod
	config.CalendarTimezone = revision.CalendarTimezone
	config.RevisionEffectiveFrom = revision.EffectiveFrom
	base := proto.Clone(req.Base).(*alertsv1.AlertRecipeBaseV1)
	base.Scope = sli.CurrentRevision.Scope
	return s.createAlert(ctx, base, "slo_burn", &alertsv1.AlertDetectorConfigV1{Config: &alertsv1.AlertDetectorConfigV1_SloBurn{SloBurn: config}})
}
func (s *alertTestServer) CreateAdvancedSignalAlert(ctx context.Context, req *alertsv1.CreateAdvancedSignalAlertRequest) (*alertsv1.CreateAlertDefinitionResponse, error) {
	return s.createAlert(ctx, req.Base, "advanced_signal", &alertsv1.AlertDetectorConfigV1{Config: &alertsv1.AlertDetectorConfigV1_AdvancedSignal{AdvancedSignal: req.Condition}})
}
func (s *alertTestServer) DeleteDefinition(ctx context.Context, req *alertsv1.DeleteAlertDefinitionRequest) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.definitions, req.Id)
	return &alertsv1.AlertMutationResponse{Ok: true, ResourceId: req.Id}, nil
}
func (s *alertTestServer) ArchiveDefinitionV2(ctx context.Context, req *alertsv1.ArchiveAlertDefinitionV2Request) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.definitions[req.DefinitionId]
	if item == nil {
		return nil, status.Error(codes.NotFound, "alert")
	}
	if item.CurrentRevisionId != req.ExpectedRevisionId {
		return nil, status.Error(codes.Aborted, "stale alert revision")
	}
	delete(s.definitions, req.DefinitionId)
	return &alertsv1.AlertMutationResponse{Ok: true, ResourceId: req.DefinitionId}, nil
}
func (s *alertTestServer) GetDefinition(ctx context.Context, req *alertsv1.GetAlertDefinitionRequest) (*alertsv1.AlertDefinitionV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.definitions[req.Id] == nil {
		return nil, status.Error(codes.NotFound, "alert")
	}
	return s.definitions[req.Id], nil
}
func (s *alertTestServer) UpdateObserve(ctx context.Context, req *alertsv1.UpdateObserveAlertRequest) (*alertsv1.AlertDefinitionV1, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.definitions[req.DefinitionId]
	if item == nil {
		return nil, status.Error(codes.NotFound, "alert")
	}
	item.Name = req.Name
	item.Description = req.Description
	item.Owner = req.Owner
	item.Action = req.Action
	item.EvaluationSettings = req.EvaluationSettings
	if req.EvaluationIntervalSeconds != nil {
		item.EvaluationIntervalSeconds = req.GetEvaluationIntervalSeconds()
	}
	item.SampleGuard = req.SampleGuard
	item.CurrentRevisionId = "revision-updated"
	return proto.Clone(item).(*alertsv1.AlertDefinitionV1), nil
}
func (s *alertTestServer) Pause(ctx context.Context, req *alertsv1.PauseAlertRequest) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.definitions[req.DefinitionId].Mode = alertsv1.AlertModeV1_ALERT_MODE_V1_DISABLED
	return &alertsv1.AlertMutationResponse{Ok: true}, nil
}
func (s *alertTestServer) Resume(ctx context.Context, req *alertsv1.ResumeAlertRequest) (*alertsv1.AlertMutationResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.definitions[req.DefinitionId].Mode = alertsv1.AlertModeV1_ALERT_MODE_V1_OBSERVE
	return &alertsv1.AlertMutationResponse{Ok: true}, nil
}
func (s *alertTestServer) PreviewAlert(ctx context.Context, req *alertsv1.PreviewAlertRequest) (*alertsv1.PreviewAlertResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	return &alertsv1.PreviewAlertResponse{Preview: &alertsv1.AlertPreviewRunV1{Id: "preview-1", DefinitionId: req.DefinitionId, PredictedFiringCount: 2, PredictedNotificationCount: 0, Result: &alertsv1.AlertPreviewResultV1{Status: "complete"}}}, nil
}

func testClient(t *testing.T, token string) (*client.Client, func()) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	service := newAlertTestServer()
	alertsv1.RegisterAlertDefinitionServiceServer(grpcServer, service)
	alertsv1.RegisterAlertRuntimeServiceServer(grpcServer, service)
	alertsv1.RegisterAlertNotificationServiceServer(grpcServer, service)
	alertsv1.RegisterAlertPreviewServiceServer(grpcServer, service)
	alertsv1.RegisterAlertSloServiceServer(grpcServer, service)
	go func() { _ = grpcServer.Serve(listener) }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	c := client.FromConn(conn, client.Config{Token: token, TenantID: testTenantID, OrgID: testOrgID, RequestTimeout: time.Second})
	return c, func() { _ = c.Close(); grpcServer.Stop(); _ = listener.Close() }
}

func TestControlledGRPCLifecycle(t *testing.T) {
	c, cleanup := testClient(t, "test-token")
	defer cleanup()
	ctx, cancel := c.Context(context.Background())
	defer cancel()
	config := &alertsv1.AlertDestinationConfigV1{Config: &alertsv1.AlertDestinationConfigV1_Webhook{Webhook: &alertsv1.AlertWebhookDestinationConfigV1{Url: "https://hooks.example.test", Method: alertsv1.AlertWebhookMethodV1_ALERT_WEBHOOK_METHOD_V1_POST}}}
	refs := &alertsv1.AlertDestinationSecretRefsV1{Authorization: "secret:webhook"}
	key := idempotency.Key(c.TenantID(), c.OrgID(), "destination", "create", "primary")
	request := &alertsv1.UpsertDestinationRequest{DestinationKey: "primary", Name: "Primary", Kind: alertsv1.AlertDestinationKindV1_ALERT_DESTINATION_KIND_V1_WEBHOOK, Enabled: true, Config: config, SecretRefs: refs, IdempotencyKey: key}
	firstDestination, err := c.Notifications.CreateDestination(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	secondDestination, err := c.Notifications.CreateDestination(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if firstDestination.Id != secondDestination.Id {
		t.Fatal("idempotent destination create changed id")
	}
	request.Id = firstDestination.Id
	request.Name = "Updated"
	if _, err = c.Notifications.UpdateDestination(ctx, request); err != nil {
		t.Fatal(err)
	}
	readDestination, err := c.Notifications.GetDestination(ctx, &alertsv1.GetAlertResourceRequest{Id: firstDestination.Id, OrgId: testOrgID})
	if err != nil {
		t.Fatal(err)
	}
	if readDestination.Name != "Updated" {
		t.Fatal("authoritative destination read failed for import")
	}
	if _, err = c.Notifications.DeleteDestination(ctx, &alertsv1.DeleteDestinationRequest{Id: firstDestination.Id, IdempotencyKey: "delete"}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Notifications.GetDestination(ctx, &alertsv1.GetAlertResourceRequest{Id: firstDestination.Id, OrgId: testOrgID}); status.Code(err) != codes.NotFound {
		t.Fatal("deleted destination remained in drift lookup")
	}
	routes := &alertsv1.AlertNotificationPolicyConfigV1{Routes: []*alertsv1.AlertNotificationRouteV1{{RouteKey: "primary", Enabled: true, Target: &alertsv1.AlertNotificationRouteTargetV1{Target: &alertsv1.AlertNotificationRouteTargetV1_DestinationKey{DestinationKey: "primary"}}, Behavior: alertsv1.AlertNotificationRouteBehaviorV1_ALERT_NOTIFICATION_ROUTE_BEHAVIOR_V1_STOP}}}
	policy, err := c.Notifications.CreateNotificationPolicy(ctx, &alertsv1.UpsertNotificationPolicyRequest{PolicyKey: "default", Name: "Default", Enabled: true, Config: routes, IdempotencyKey: "policy-create"})
	if err != nil {
		t.Fatal(err)
	}
	policy.Name = "Updated"
	expectedPolicyRevision := policy.Revision
	if _, err = c.Notifications.UpdateNotificationPolicy(ctx, &alertsv1.UpsertNotificationPolicyRequest{Id: policy.Id, PolicyKey: policy.PolicyKey, Name: policy.Name, Enabled: true, Config: routes, IdempotencyKey: "policy-update", ExpectedRevision: &expectedPolicyRevision}); err != nil {
		t.Fatal(err)
	}
	readPolicy, err := c.Notifications.GetNotificationPolicy(ctx, &alertsv1.GetAlertResourceRequest{Id: policy.Id, OrgId: testOrgID})
	if err != nil || readPolicy.Name != "Updated" {
		t.Fatal("policy import lookup failed")
	}
	if _, err = c.Notifications.DeleteNotificationPolicy(ctx, &alertsv1.DeleteOperatorResourceRequest{OrgId: testOrgID, Id: policy.Id, IdempotencyKey: "policy-delete"}); err != nil {
		t.Fatal(err)
	}
	base := &alertsv1.AlertRecipeBaseV1{OrgId: testOrgID, Slug: "quality", Name: "Quality", Severity: alertsv1.AlertSeverityV1_ALERT_SEVERITY_V1_WARNING, Scope: &alertsv1.AlertScopeV1{}, Owner: &alertsv1.AlertOwnerRefV1{}, Action: &alertsv1.AlertActionV1{}, EvaluationSettings: &alertsv1.AlertEvaluationSettingsV1{}, SampleGuard: &alertsv1.AlertSampleGuardV1{}, IdempotencyKey: "alert-quality"}
	quality, err := c.Definitions.CreateAgentQualityRegressionAlert(ctx, &alertsv1.CreateAgentQualityRegressionAlertRequest{Base: base, RecipeConfig: &alertsv1.AgentQualityRegressionConfigV1{}})
	if err != nil {
		t.Fatal(err)
	}
	base.IdempotencyKey = "alert-cost"
	if _, err = c.Definitions.CreateCostPerSuccessAlert(ctx, &alertsv1.CreateCostPerSuccessAlertRequest{Base: base, RecipeConfig: &alertsv1.CostPerSuccessConfigV1{}}); err != nil {
		t.Fatal(err)
	}
	base.IdempotencyKey = "alert-slo"
	if _, err = c.Definitions.CreateSloBurnAlert(ctx, &alertsv1.CreateSloBurnAlertRequest{Base: base, RecipeConfig: &alertsv1.SloBurnConfigV1{}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SLO burn alert without slo_id: got %v, want invalid argument", err)
	}
	base.IdempotencyKey = "alert-symptom"
	if _, err = c.Definitions.CreateAdvancedSignalAlert(ctx, &alertsv1.CreateAdvancedSignalAlertRequest{Base: base, Condition: &alertsv1.AdvancedSignalConfigV1{}}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Definitions.UpdateObserve(ctx, &alertsv1.UpdateObserveAlertRequest{DefinitionId: quality.Definition.Id, Name: "Updated quality", Owner: &alertsv1.AlertOwnerRefV1{}, Action: &alertsv1.AlertActionV1{}, EvaluationSettings: &alertsv1.AlertEvaluationSettingsV1{}, SampleGuard: &alertsv1.AlertSampleGuardV1{}, IdempotencyKey: "alert-update"}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Runtime.Pause(ctx, &alertsv1.PauseAlertRequest{DefinitionId: quality.Definition.Id, IdempotencyKey: "pause"}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Runtime.Resume(ctx, &alertsv1.ResumeAlertRequest{DefinitionId: quality.Definition.Id, RevisionId: "revision-1", IdempotencyKey: "resume"}); err != nil {
		t.Fatal(err)
	}
	preview, err := c.Previews.PreviewAlert(ctx, &alertsv1.PreviewAlertRequest{DefinitionId: quality.Definition.Id, IdempotencyKey: "preview"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Preview.PredictedNotificationCount != 0 {
		t.Fatal("Observe preview predicted notifications")
	}
	if _, err = c.Definitions.DeleteDefinition(ctx, &alertsv1.DeleteAlertDefinitionRequest{Id: quality.Definition.Id, IdempotencyKey: "alert-delete"}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Definitions.GetDefinition(ctx, &alertsv1.GetAlertDefinitionRequest{Id: quality.Definition.Id}); status.Code(err) != codes.NotFound {
		t.Fatal("deleted alert remained readable")
	}
}

func TestControlledGRPCPermissionDenied(t *testing.T) {
	c, cleanup := testClient(t, "wrong-token")
	defer cleanup()
	ctx, cancel := c.Context(context.Background())
	defer cancel()
	_, err := c.Notifications.ListDestinations(ctx, &alertsv1.ListDestinationsRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want permission denied", err)
	}
}
