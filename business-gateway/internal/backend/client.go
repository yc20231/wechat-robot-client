package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBody = 4 << 20

type InventoryQuery struct {
	CustomerCode string
	Keyword      string
	ProductCode  string
	Limit        int
}

type InventorySummary struct {
	Count          int     `json:"count"`
	TotalCartonQty float64 `json:"total_carton_qty"`
	TotalWeightJin float64 `json:"total_weight_jin"`
}

type InventoryItem struct {
	CustomerName    string  `json:"customer_name"`
	ProductCode     string  `json:"product_code"`
	ProductName     string  `json:"product_name"`
	Color           string  `json:"color"`
	PatternDesc     string  `json:"pattern_desc"`
	Specification   string  `json:"specification"`
	CartonQty       float64 `json:"carton_qty"`
	UnitQty         float64 `json:"unit_qty"`
	Unit            string  `json:"unit"`
	WeightPerCarton float64 `json:"weight_per_carton"`
	TotalWeightJin  float64 `json:"total_weight_jin"`
}

type Inventory struct {
	CustomerCode string           `json:"customer_code"`
	CustomerName string           `json:"customer_name"`
	Summary      InventorySummary `json:"summary"`
	Items        []InventoryItem  `json:"items"`
}

type Service interface {
	QueryInventory(ctx context.Context, query InventoryQuery) (Inventory, error)
	Health(ctx context.Context) error
}

type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil {
		return nil, fmt.Errorf("解析 BACKEND_URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("BACKEND_URL 必须是有效的 http/https 地址")
	}
	return &Client{
		baseURL: parsed.String(),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) QueryInventory(ctx context.Context, query InventoryQuery) (Inventory, error) {
	params := url.Values{}
	params.Set("customer_code", query.CustomerCode)
	if query.Keyword != "" {
		params.Set("keyword", query.Keyword)
	}
	if query.ProductCode != "" {
		params.Set("product_code", query.ProductCode)
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	params.Set("limit", strconv.Itoa(limit))

	var response struct {
		Code    int       `json:"code"`
		Message string    `json:"message"`
		Data    Inventory `json:"data"`
	}
	if err := c.get(ctx, "/api/bot/inventory", params, &response); err != nil {
		return Inventory{}, err
	}
	if response.Code != 0 {
		return Inventory{}, fmt.Errorf("后端拒绝库存查询: %s", response.Message)
	}
	return response.Data, nil
}

func (c *Client) Health(ctx context.Context) error {
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := c.get(ctx, "/api/bot/health", nil, &response); err != nil {
		return err
	}
	if response.Code != 0 {
		return fmt.Errorf("后端健康检查失败: %s", response.Message)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, target any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建后端请求: %w", err)
	}
	req.Header.Set("X-Bot-Token", c.token)
	// 兼容现有 ThinkPHP BotTokenMiddleware 对调用来源的限制。
	req.Header.Set("User-Agent", "bot-mcp/business-gateway-1.0")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("调用业务后端: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return fmt.Errorf("读取业务后端响应: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("业务后端返回 HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("解析业务后端响应: %w", err)
	}
	return nil
}
