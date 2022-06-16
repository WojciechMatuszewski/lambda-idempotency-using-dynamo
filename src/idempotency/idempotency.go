package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	dynamodbattributevalue "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	dynamodbexpression "github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Key struct {
	PK string `dynamodbav:"PK"`
}

type Item[R comparable] struct {
	Key

	TTL int64 `dynamodbav:"TTL"`

	Status string `dynamodbav:"status"`
	Result R      `dynamodbav:"result"`
}

var ErrPersistanceLayer = errors.New("persistance layer error")

type Callback[R comparable] func() (R, error)

func Idempotent[R comparable](ctx context.Context, key string, tableName string, callback Callback[R]) (R, error) {
	var zero R

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return zero, err
	}
	ddb := dynamodb.NewFromConfig(cfg)

	item, err := saveInProgress[R](ctx, key, tableName, ddb)
	if err != nil {
		return zero, err
	}

	fmt.Println("[Idempotent] item", item, item.Result != zero)

	if item.Result != zero {
		return item.Result, nil
	}

	result, err := callback()

	fmt.Println("[Idempotent] result", result)

	if err != nil {
		delErr := delete(ctx, key, tableName, ddb)
		if delErr != nil {
			return zero, delErr
		}

		return zero, err
	}

	fmt.Println("[Idempotent] updating with the result", result)
	err = update(ctx, result, key, tableName, ddb)
	if err != nil {
		return zero, err
	}

	return result, nil
}

func saveInProgress[R comparable](ctx context.Context, key string, tableName string, client *dynamodb.Client) (Item[R], error) {
	nowTTL := time.Now().Unix()

	fmt.Println("[saveInProgress] building the expression")
	expr, err := dynamodbexpression.NewBuilder().WithCondition(
		dynamodbexpression.AttributeNotExists(dynamodbexpression.Name("PK")),
	).Build()
	if err != nil {
		return Item[R]{}, ErrPersistanceLayer
	}

	item := Item[R]{
		Key: Key{
			PK: key,
		},
		Status: "IN_PROGRESS",
		TTL:    nowTTL + 360,
	}
	itemAvs, err := dynamodbattributevalue.MarshalMap(item)
	if err != nil {
		return Item[R]{}, fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
	}
	fmt.Println("[saveInProgress] putting into DynamoDB")
	_, err = client.PutItem(ctx, &dynamodb.PutItemInput{
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Item:                      itemAvs,
		TableName:                 aws.String(tableName),
	})
	if err == nil {
		fmt.Println("[saveInProgress] returning the result, put OK!")
		return item, nil
	}

	fmt.Println("[saveInProgress] checking errors from put", err.Error())
	var conditionFailedErr *dynamodbtypes.ConditionalCheckFailedException
	if errors.As(err, &conditionFailedErr) {
		fmt.Println("[saveInProgress] conditional error, getting the item")
		got, getErr := get[R](ctx, key, tableName, client)
		if getErr != nil {
			fmt.Println("[saveInProgress] item get failed", getErr.Error())
			return Item[R]{}, fmt.Errorf("%s: %w", getErr.Error(), ErrPersistanceLayer)
		}

		if got.Status == "IN_PROGRESS" {
			fmt.Println("[saveInProgress] item in progress, failing")
			return Item[R]{}, fmt.Errorf("in progress!: %w", ErrPersistanceLayer)
		}

		if got.TTL < nowTTL {
			fmt.Println("[saveInProgress] delete and return, returning")
			delErr := delete(ctx, key, tableName, client)
			if delErr != nil {
				fmt.Println("[saveInProgress] delete error", delErr.Error())
			}

			return item, nil
		}

		fmt.Println("[saveInProgress] returning the item")
		return got, nil
	}

	fmt.Println("[saveInProgress] not a condition error, returning error")
	return Item[R]{}, fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
}

var errItemNotFound = errors.New("Item not found")

func get[R comparable](ctx context.Context, key string, tableName string, client *dynamodb.Client) (Item[R], error) {
	itemKey := Key{PK: key}
	itemKeyAvs, err := dynamodbattributevalue.MarshalMap(itemKey)
	fmt.Println("[get] itemKeyAvs", itemKeyAvs)
	if err != nil {
		fmt.Println("[get] getting from DynamoDB", err.Error())
		return Item[R]{}, fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
	}
	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		Key:            itemKeyAvs,
		TableName:      aws.String(tableName),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		fmt.Println("[get] error from DynamoDB", err.Error())
		return Item[R]{}, fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
	}

	if out.Item == nil {
		fmt.Println("[get] item not found")
		return Item[R]{}, errItemNotFound
	}

	var idempotencyItem Item[R]
	err = dynamodbattributevalue.UnmarshalMap(out.Item, &idempotencyItem)
	if err != nil {
		return Item[R]{}, fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
	}

	fmt.Println("[get] found! returning")
	return idempotencyItem, nil
}

func update[R comparable](ctx context.Context, result R, key string, tableName string, client *dynamodb.Client) error {
	keyAvs, err := dynamodbattributevalue.MarshalMap(Key{PK: key})
	if err != nil {
		return fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
	}

	expr, err := dynamodbexpression.
		NewBuilder().
		WithUpdate(
			dynamodbexpression.
				Set(dynamodbexpression.Name("result"), dynamodbexpression.Value(result)).
				Set(dynamodbexpression.Name("status"), dynamodbexpression.Value("COMPLETED")),
		).
		Build()
	if err != nil {
		return fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
	}

	fmt.Println("[update] updating in DynamoDB")
	_, err = client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(tableName),
		Key:                       keyAvs,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		fmt.Println("[update] error from DynamoDB", err.Error())
		return fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
	}

	return nil
}

func delete(ctx context.Context, key string, tableName string, client *dynamodb.Client) error {
	itemKey := Key{PK: key}
	itemKeyAvs, err := dynamodbattributevalue.MarshalMap(itemKey)
	if err != nil {
		return fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
	}

	fmt.Println("[delete] deleting")
	_, err = client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key:       itemKeyAvs,
	})
	if err != nil {
		fmt.Println("[delete] error from DynamoDB", err.Error())
		return fmt.Errorf("%s: %w", err.Error(), ErrPersistanceLayer)
	}

	fmt.Println("[delete] all good, returning")
	return nil
}
