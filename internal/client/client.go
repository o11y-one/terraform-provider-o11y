package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"time"

	alertsv1 "github.com/o11y-one/terraform-provider-o11y/internal/gen/proto/o11y_one/alerts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Config struct {
	Endpoint           string
	Token              string
	TenantID           string
	OrgID              string
	InsecureSkipVerify bool
	ConnectTimeout     time.Duration
	RequestTimeout     time.Duration
	DialOptions        []grpc.DialOption
}

type Client struct {
	Conn           *grpc.ClientConn
	Definitions    alertsv1.AlertDefinitionServiceClient
	Runtime        alertsv1.AlertRuntimeServiceClient
	Notifications  alertsv1.AlertNotificationServiceClient
	Previews       alertsv1.AlertPreviewServiceClient
	token          string
	tenantID       string
	orgID          string
	requestTimeout time.Duration
}

func New(ctx context.Context, config Config) (*Client, error) {
	u, err := url.Parse(config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}
	options := append([]grpc.DialOption{}, config.DialOptions...)
	if len(config.DialOptions) == 0 {
		if u.Scheme == "http" {
			options = append(options, grpc.WithTransportCredentials(insecure.NewCredentials()))
		} else {
			tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname(), InsecureSkipVerify: config.InsecureSkipVerify} //nolint:gosec -- explicit provider option
			options = append(options, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		}
	}
	connectCtx, cancel := context.WithTimeout(ctx, config.ConnectTimeout)
	defer cancel()
	options = append(options, grpc.WithBlock())
	conn, err := grpc.DialContext(connectCtx, u.Host, options...)
	if err != nil {
		return nil, fmt.Errorf("connect to gRPC endpoint: %w", err)
	}
	return FromConn(conn, config), nil
}

func FromConn(conn *grpc.ClientConn, config Config) *Client {
	return &Client{
		Conn: conn, Definitions: alertsv1.NewAlertDefinitionServiceClient(conn),
		Runtime: alertsv1.NewAlertRuntimeServiceClient(conn), Notifications: alertsv1.NewAlertNotificationServiceClient(conn),
		Previews: alertsv1.NewAlertPreviewServiceClient(conn), token: config.Token, tenantID: config.TenantID,
		orgID: config.OrgID, requestTimeout: config.RequestTimeout,
	}
}

func (c *Client) Context(ctx context.Context) (context.Context, context.CancelFunc) {
	values := []string{"x-o11y-key", c.token, "x-o11y-tenant-id", c.tenantID}
	if c.orgID != "" {
		values = append(values, "x-o11y-org-id", c.orgID)
	}
	return context.WithTimeout(metadata.NewOutgoingContext(ctx, metadata.Pairs(values...)), c.requestTimeout)
}

func (c *Client) TenantID() string { return c.tenantID }
func (c *Client) OrgID() string    { return c.orgID }

func (c *Client) Close() error {
	if c.Conn == nil {
		return nil
	}
	return c.Conn.Close()
}
