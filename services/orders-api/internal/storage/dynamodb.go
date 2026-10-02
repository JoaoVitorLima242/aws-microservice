package storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/shopspring/decimal"

	"shop-aws/services/orders-api/internal/orders"
)

// DynamoDB stores orders in a table whose partition key is "id" (string).
type DynamoDB struct {
	client *dynamodb.Client
	table  string
}

func NewDynamoDB(client *dynamodb.Client, table string) *DynamoDB {
	return &DynamoDB{client: client, table: table}
}

func (d *DynamoDB) Create(ctx context.Context, o orders.Order) error {
	_, err := d.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(d.table),
		Item:                toItem(o),
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	if err != nil {
		return fmt.Errorf("dynamodb put item: %w", err)
	}
	return nil
}

func (d *DynamoDB) Get(ctx context.Context, id string) (orders.Order, error) {
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(d.table),
		Key:            key(id),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return orders.Order{}, fmt.Errorf("dynamodb get item: %w", err)
	}
	if out.Item == nil {
		return orders.Order{}, orders.ErrNotFound
	}
	return fromItem(out.Item)
}

// List scans the whole table. Fine for a demo-sized table; a real system
// would paginate the API or query a GSI instead.
func (d *DynamoDB) List(ctx context.Context) ([]orders.Order, error) {
	var list []orders.Order
	p := dynamodb.NewScanPaginator(d.client, &dynamodb.ScanInput{TableName: aws.String(d.table)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("dynamodb scan: %w", err)
		}
		for _, item := range page.Items {
			o, err := fromItem(item)
			if err != nil {
				return nil, err
			}
			list = append(list, o)
		}
	}
	if list == nil {
		list = []orders.Order{}
	}
	sortNewestFirst(list)
	return list, nil
}

func (d *DynamoDB) MarkEventPublished(ctx context.Context, id string) error {
	_, err := d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(d.table),
		Key:                 key(id),
		UpdateExpression:    aws.String("SET eventPublished = :true"),
		ConditionExpression: aws.String("attribute_exists(id)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true": &types.AttributeValueMemberBOOL{Value: true},
		},
	})
	var condErr *types.ConditionalCheckFailedException
	if errors.As(err, &condErr) {
		return orders.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("dynamodb update item: %w", err)
	}
	return nil
}

func key(id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: id}}
}

// Money is stored as DynamoDB numbers (N) to keep decimal precision.
func toItem(o orders.Order) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"id":             &types.AttributeValueMemberS{Value: o.ID},
		"productId":      &types.AttributeValueMemberS{Value: o.ProductID},
		"quantity":       &types.AttributeValueMemberN{Value: strconv.Itoa(o.Quantity)},
		"unitPrice":      &types.AttributeValueMemberN{Value: o.UnitPrice.String()},
		"total":          &types.AttributeValueMemberN{Value: o.Total.String()},
		"customerEmail":  &types.AttributeValueMemberS{Value: o.CustomerEmail},
		"status":         &types.AttributeValueMemberS{Value: o.Status},
		"createdAt":      &types.AttributeValueMemberS{Value: o.CreatedAt.UTC().Format(time.RFC3339Nano)},
		"eventPublished": &types.AttributeValueMemberBOOL{Value: o.EventPublished},
	}
}

func fromItem(item map[string]types.AttributeValue) (orders.Order, error) {
	r := itemReader{item: item}
	o := orders.Order{
		ID:             r.str("id"),
		ProductID:      r.str("productId"),
		CustomerEmail:  r.str("customerEmail"),
		Status:         r.str("status"),
		UnitPrice:      r.dec("unitPrice"),
		Total:          r.dec("total"),
		EventPublished: r.boolean("eventPublished"),
	}
	o.Quantity = int(r.dec("quantity").IntPart())
	if createdAt := r.str("createdAt"); r.err == nil {
		t, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			r.err = fmt.Errorf("attribute createdAt: %w", err)
		}
		o.CreatedAt = t
	}
	if r.err != nil {
		return orders.Order{}, fmt.Errorf("decode order item %q: %w", o.ID, r.err)
	}
	return o, nil
}

// itemReader reads typed attributes and remembers the first error.
type itemReader struct {
	item map[string]types.AttributeValue
	err  error
}

func (r *itemReader) fail(name, want string) {
	if r.err == nil {
		r.err = fmt.Errorf("attribute %s: missing or not of type %s", name, want)
	}
}

func (r *itemReader) str(name string) string {
	v, ok := r.item[name].(*types.AttributeValueMemberS)
	if !ok {
		r.fail(name, "S")
		return ""
	}
	return v.Value
}

func (r *itemReader) dec(name string) decimal.Decimal {
	v, ok := r.item[name].(*types.AttributeValueMemberN)
	if !ok {
		r.fail(name, "N")
		return decimal.Zero
	}
	d, err := decimal.NewFromString(v.Value)
	if err != nil {
		r.fail(name, "N")
	}
	return d
}

func (r *itemReader) boolean(name string) bool {
	v, ok := r.item[name].(*types.AttributeValueMemberBOOL)
	if !ok {
		r.fail(name, "BOOL")
		return false
	}
	return v.Value
}
