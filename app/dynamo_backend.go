package app

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

func BuildDynamodbClient() *dynamodb.DynamoDB {
	// Initialize a session that the SDK will use to load
	// credentials from the shared credentials file ~/.aws/credentials
	// and region from the shared configuration file ~/.aws/config.
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	cfg := aws.Config{
		Endpoint: aws.String("http://localhost:8000"),
		Region:   aws.String("eu-west-2"),
	}

	// Create DynamoDB client
	svc := dynamodb.New(sess, &cfg)
	return svc
}

type Backend interface {
	Acquire(name string, nonce string, duration time.Duration) (*Acquisition, error)
	UpdateValue(name string, fence int64, extension time.Duration, value string) error
	Heartbeat(name string, fence int64, extension time.Duration) error
	Release(name string, fence int64) error
}

type DynamoBackend struct {
	Db    *dynamodb.DynamoDB
	Table string
}

type Acquisition struct {
	AcquireTime time.Time `json:"acquireTime"`
	ExpireTime  time.Time `json:"expireTime"`
	Fence       int64     `json:"fence"`
	Body        string    `json:"body"`
}

func (t *DynamoBackend) EnsureTableExists() error {
	_, err := t.Db.CreateTable(&dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: awsString("LockName"),
				AttributeType: awsString("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: awsString("LockName"),
				KeyType:       awsString("HASH"),
			},
		},
		BillingMode: awsString("PAY_PER_REQUEST"),
		TableName:   awsString(t.Table),
	})
	return err
}

type ExpectedError struct {
	Cause error
}

func (e ExpectedError) Error() string {
	return e.Cause.Error()
}

func (t *DynamoBackend) Acquire(name string, nonce string, duration time.Duration) (*Acquisition, error) {
	now := time.Now()
	expiry := now.Add(duration)

	rec, err := t.Db.UpdateItem(&dynamodb.UpdateItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"LockName": {S: awsString(name)},
		},
		TableName:           awsString(t.Table),
		ConditionExpression: awsString("attribute_not_exists(LockName) OR ExpireTime < :now OR Nonce = :nonce"),
		UpdateExpression:    awsString("SET Nonce = :nonce, Fence = if_not_exists(Fence, :zero) + :one, AcquireTime = :now, HeartbeatTime = :now, ExpireTime = :expire"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":one":    {N: awsInt(1)},
			":zero":   {N: awsInt(0)},
			":now":    {N: awsTime(now)},
			":expire": {N: awsTime(expiry)},
			":nonce":  {S: awsString(nonce)},
		},
		ReturnValues: awsString("ALL_NEW"),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == dynamodb.ErrCodeConditionalCheckFailedException {
				return nil, &ExpectedError{errors.New("Lock in use")}
			}
		}
		return nil, err
	}

	acqTime, err := extractInt(rec.Attributes, "AcquireTime")
	if err != nil {
		return nil, err
	}

	expTime, err := extractInt(rec.Attributes, "ExpireTime")
	if err != nil {
		return nil, err
	}

	fence, err := extractInt(rec.Attributes, "Fence")
	if err != nil {
		return nil, err
	}

	body, _ := extractString(rec.Attributes, "Body")

	return &Acquisition{
		AcquireTime: time.Unix(0, acqTime),
		ExpireTime:  time.Unix(0, expTime),
		Fence:       fence,
		Body:        body,
	}, nil
}

func extractString(attrs map[string]*dynamodb.AttributeValue, key string) (string, error) {
	val := attrs[key]
	if val == nil || val.S == nil {
		return "", fmt.Errorf("No string %s in attributes", key)
	}
	return *val.S, nil
}

func extractInt(attrs map[string]*dynamodb.AttributeValue, key string) (int64, error) {
	val := attrs[key]
	if val == nil || val.N == nil {
		return 0, fmt.Errorf("No int %s in attributes", key)
	}
	valInt, err := strconv.ParseInt(*val.N, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("Error converting %s to integer: %v", key, err)
	}
	return valInt, err
}

func (t *DynamoBackend) Heartbeat(name string, fence int64, extension time.Duration) error {
	now := time.Now()
	expire := now.Add(extension)

	_, err := t.Db.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: aws.String(t.Table),
		Key: map[string]*dynamodb.AttributeValue{
			"LockName": {S: awsString(name)},
		},
		ConditionExpression: awsString("attribute_exists(LockName) AND ExpireTime > :now AND Fence = :fence"),
		UpdateExpression:    awsString("SET HeartbeatTime :now, ExpireTime = :expire"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":fence":  {N: awsInt(fence)},
			":now":    {N: awsTime(now)},
			":expire": {N: awsTime(expire)},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == dynamodb.ErrCodeConditionalCheckFailedException {
				return &ExpectedError{errors.New("Lock expired")}
			}
		}
		return err
	}

	return nil
}

func (t *DynamoBackend) UpdateValue(name string, fence int64, extension time.Duration, value string) error {
	now := time.Now()
	expire := now.Add(extension)

	_, err := t.Db.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: aws.String(t.Table),
		Key: map[string]*dynamodb.AttributeValue{
			"LockName": {S: awsString(name)},
		},
		ConditionExpression: awsString("attribute_exists(LockName) AND ExpireTime > :now AND Fence = :fence"),
		UpdateExpression:    awsString("SET Body = :value, HeartbeatTime = :now, ExpireTime = :expire"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":now":    {N: awsTime(now)},
			":expire": {N: awsTime(expire)},
			":fence":  {N: awsInt(fence)},
			":value":  {S: awsString(value)},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == dynamodb.ErrCodeConditionalCheckFailedException {
				return &ExpectedError{errors.New("Lock expired")}
			}
		}
		return err
	}
	return nil
}

func (t *DynamoBackend) Release(name string, fence int64) error {
	_, err := t.Db.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: aws.String(t.Table),
		Key: map[string]*dynamodb.AttributeValue{
			"LockName": {S: awsString(name)},
		},
		ConditionExpression: awsString("attribute_exists(LockName) AND Fence = :fence"),
		UpdateExpression:    awsString("SET ExpireTime = :zero"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":zero": {N: awsInt(0)},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == dynamodb.ErrCodeConditionalCheckFailedException {
				// Idempotent release -- any release will succeed, even if it was never
				// held, or the fence is wrong.
				return nil
			}
		}
		return err
	}
	return nil
}

func awsTime(t time.Time) *string {
	return aws.String(fmt.Sprintf("%d", t.UnixNano()))
}

func awsInt(v int64) *string {
	return aws.String(fmt.Sprintf("%d", v))
}

func awsString(s string) *string {
	return aws.String(s)
}
