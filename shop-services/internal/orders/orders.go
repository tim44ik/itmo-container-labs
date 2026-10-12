package orders

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"simpleapi/telemetry"
)

type Order struct {
	ID          int64      `json:"id"`
	Item        string     `json:"item"`
	Quantity    int        `json:"quantity"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at"`
}

type request struct {
	Item     string `json:"item"`
	Quantity int    `json:"quantity"`
}
type API struct{ DB *sql.DB }

func (a API) Create(w http.ResponseWriter, r *http.Request) {
	a.serve(w, r, "/order", http.MethodPost, a.create)
}
func (a API) List(w http.ResponseWriter, r *http.Request) {
	a.serve(w, r, "/orders", http.MethodGet, a.list)
}

func (a API) serve(w http.ResponseWriter, r *http.Request, path, method string, fn func(context.Context, http.ResponseWriter, *http.Request) (int, error)) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := otel.Tracer("shop-api").Start(ctx, r.Method+" "+path, trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := time.Now()
	status := http.StatusMethodNotAllowed
	var err error
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
	if r.Method != method {
		w.Header().Set("Allow", method)
		err = errors.New("method not allowed")
	} else {
		status, err = fn(ctx, w, r)
	}
	level := "INFO"
	message := "request completed"
	if err != nil {
		message = err.Error()
		if status >= 500 {
			level = "ERROR"
			span.RecordError(err)
			span.SetStatus(codes.Error, "request failed")
		}
		w.WriteHeader(status)
		// Do not expose database errors or connection details to the caller.
		public := "invalid request"
		if status >= 500 {
			public = "database operation failed"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"error": public})
	}
	telemetry.HttpRequestsTotal.WithLabelValues(path, strconv.Itoa(status)).Inc()
	telemetry.HttpDurationHistogram.WithLabelValues(path).Observe(time.Since(start).Seconds())
	telemetry.WriteLog(level, message, path, span.SpanContext().TraceID().String())
}

func (a API) create(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var input request
	if err := dec.Decode(&input); err != nil {
		return 400, errors.New("invalid JSON body")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return 400, errors.New("expected one JSON object")
	}
	input.Item = strings.TrimSpace(input.Item)
	if input.Item == "" || len([]rune(input.Item)) > 200 || input.Quantity < 1 || input.Quantity > 1000 {
		return 400, errors.New("item must be 1..200 characters and quantity 1..1000")
	}
	dbctx, span := otel.Tracer("shop-api").Start(ctx, "postgres.insert_order", trace.WithSpanKind(trace.SpanKindClient))
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	var order Order
	err := a.DB.QueryRowContext(dbctx, `INSERT INTO orders(item,quantity,traceparent) VALUES($1,$2,$3) RETURNING id,item,quantity,status,created_at,processed_at`, input.Item, input.Quantity, carrier.Get("traceparent")).Scan(&order.ID, &order.Item, &order.Quantity, &order.Status, &order.CreatedAt, &order.ProcessedAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "insert failed")
	}
	span.End()
	if err != nil {
		return 503, err
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(order)
	return http.StatusCreated, nil
}

func (a API) list(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000 {
			return 400, errors.New("limit must be 1..1000")
		}
		limit = n
	}
	ctx, span := otel.Tracer("shop-api").Start(ctx, "postgres.list_orders", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	rows, err := a.DB.QueryContext(ctx, `SELECT id,item,quantity,status,created_at,processed_at FROM orders ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "select failed")
		return 503, err
	}
	defer rows.Close()
	result := make([]Order, 0)
	for rows.Next() {
		var order Order
		if err = rows.Scan(&order.ID, &order.Item, &order.Quantity, &order.Status, &order.CreatedAt, &order.ProcessedAt); err != nil {
			return 503, err
		}
		result = append(result, order)
	}
	if err = rows.Err(); err != nil {
		return 503, err
	}
	_ = json.NewEncoder(w).Encode(result)
	return 200, nil
}
