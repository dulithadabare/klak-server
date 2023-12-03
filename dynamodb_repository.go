package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamoDbRepository struct {
	client *dynamodb.Client
}

const (
	DDB_TABLE_USER              string = "User"
	DDB_TABLE_USER_MESSAGES     string = "Messages"
	DDB_TABLE_DEVICE_TOKEN      string = "DeviceToken"
	DDB_TABLE_TASK              string = "Task"
	DDB_TABLE_CHAT_GROUP        string = "ChatGroup"
	DDB_TABLE_CHAT_GROUP_MEMBER string = "ChatGroupMember"
)

func tableExists(d *dynamodb.Client, name string) bool {
	tables, err := d.ListTables(context.TODO(), &dynamodb.ListTablesInput{})
	if err != nil {
		log.Fatal("ListTables failed", err)
	}
	for _, n := range tables.TableNames {
		if n == name {
			return true
		}
	}
	return false
}

func createUserTable(ctx context.Context, d *dynamodb.Client) (*types.TableDescription, error) {
	if tableExists(d, DDB_TABLE_USER) {
		log.Printf("table=%v already exists\n", DDB_TABLE_USER)
		return nil, nil
	}
	var tableDesc *types.TableDescription
	table, err := d.CreateTable(ctx, &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{{
			AttributeName: aws.String("userId"),
			AttributeType: types.ScalarAttributeTypeS,
		}},
		KeySchema: []types.KeySchemaElement{{
			AttributeName: aws.String("userId"),
			KeyType:       types.KeyTypeHash,
		}},
		TableName:   aws.String(DDB_TABLE_USER),
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		log.Printf("Couldn't create table %v. Here's why: %v\n", DDB_TABLE_USER, err)
	} else {
		waiter := dynamodb.NewTableExistsWaiter(d)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(DDB_TABLE_USER)}, 5*time.Minute)
		if err != nil {
			log.Printf("Wait for table exists failed. Here's why: %v\n", err)
		}
		tableDesc = table.TableDescription
	}
	return tableDesc, err
}

func (db DynamoDbRepository) addUser(ctx context.Context, user AddUserModel) error {
	item, err := attributevalue.MarshalMap(user)
	if err != nil {
		panic(err)
	}
	_, err = db.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(DDB_TABLE_USER), Item: item,
	})
	if err != nil {
		log.Printf("Couldn't add item to table. Here's why: %v\n", err)
	}
	return err
}

//TODO: pagination if scan is larger than 1MB
func (db DynamoDbRepository) getUsers(ctx context.Context) ([]AddUserModel, error) {
	var movies []AddUserModel
	var err error
	var response *dynamodb.ScanOutput
	response, err = db.client.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(DDB_TABLE_USER),
	})
	if err != nil {
		log.Printf("Couldn't scan for users. Here's why: %v\n", err)
	} else {
		err = attributevalue.UnmarshalListOfMaps(response.Items, &movies)
		if err != nil {
			log.Printf("Couldn't unmarshal query response. Here's why: %v\n", err)
		}
	}

	return movies, err
}

func (db DynamoDbRepository) getUserById(ctx context.Context, userId string) (AddUserModel, error) {
	var err error
	var response *dynamodb.QueryOutput
	var movies []AddUserModel
	keyEx := expression.Key("userId").Equal(expression.Value(userId))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("Couldn't build epxression for query. Here's why: %v\n", err)
	} else {
		response, err = db.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(DDB_TABLE_USER),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			KeyConditionExpression:    expr.KeyCondition(),
		})
		if err != nil {
			log.Printf("Couldn't query for users with id %v. Here's why: %v\n", userId, err)
		} else {
			err = attributevalue.UnmarshalListOfMaps(response.Items, &movies)
			if err != nil {
				log.Printf("Couldn't unmarshal query response. Here's why: %v\n", err)
			}
		}
	}
	return movies[0], err
}

func createMessageTable(ctx context.Context, d *dynamodb.Client) (*types.TableDescription, error) {
	if tableExists(d, DDB_TABLE_USER_MESSAGES) {
		log.Printf("table=%v already exists\n", DDB_TABLE_USER_MESSAGES)
		return nil, nil
	}
	var tableDesc *types.TableDescription
	table, err := d.CreateTable(ctx, &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{{
			AttributeName: aws.String("id"),
			AttributeType: types.ScalarAttributeTypeS,
		}, {
			AttributeName: aws.String("userId"),
			AttributeType: types.ScalarAttributeTypeS,
		}},
		KeySchema: []types.KeySchemaElement{{
			AttributeName: aws.String("userId"),
			KeyType:       types.KeyTypeHash,
		}, {
			AttributeName: aws.String("id"),
			KeyType:       types.KeyTypeRange,
		}},
		TableName:   aws.String(DDB_TABLE_USER_MESSAGES),
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		log.Printf("Couldn't create table %v. Here's why: %v\n", DDB_TABLE_USER_MESSAGES, err)
	} else {
		waiter := dynamodb.NewTableExistsWaiter(d)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(DDB_TABLE_USER_MESSAGES)}, 5*time.Minute)
		if err != nil {
			log.Printf("Wait for table exists failed. Here's why: %v\n", err)
		}
		tableDesc = table.TableDescription
	}
	return tableDesc, err
}

func (db DynamoDbRepository) addMessage(ctx context.Context, message ServerPush) error {
	item, err := attributevalue.MarshalMap(message)
	if err != nil {
		panic(err)
	}
	_, err = db.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(DDB_TABLE_USER_MESSAGES), Item: item,
	})
	if err != nil {
		log.Printf("Couldn't add item to table. Here's why: %v\n", err)
	}
	return err
}

func (db DynamoDbRepository) getMessages(ctx context.Context, userId string) ([]ServerPush, error) {
	var err error
	var response *dynamodb.QueryOutput
	var movies []ServerPush
	keyEx := expression.Key("userId").Equal(expression.Value(userId))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("Couldn't build epxression for query. Here's why: %v\n", err)
	} else {
		response, err = db.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(DDB_TABLE_USER_MESSAGES),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			KeyConditionExpression:    expr.KeyCondition(),
		})
		if err != nil {
			log.Printf("Couldn't query for movies released in %v. Here's why: %v\n", userId, err)
		} else {
			err = attributevalue.UnmarshalListOfMaps(response.Items, &movies)
			if err != nil {
				log.Printf("Couldn't unmarshal query response. Here's why: %v\n", err)
			}
		}
	}
	return movies, err
}

func (db DynamoDbRepository) getMessageById(ctx context.Context, mid string, userId string) (ServerPush, error) {
	var err error
	var response *dynamodb.QueryOutput
	var movies []ServerPush
	keyEx := expression.KeyAnd(expression.Key("userId").Equal(expression.Value(userId)), expression.Key("id").Equal(expression.Value(mid)))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("Couldn't build epxression for query. Here's why: %v\n", err)
	} else {
		response, err = db.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(DDB_TABLE_USER_MESSAGES),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			KeyConditionExpression:    expr.KeyCondition(),
		})
		if err != nil {
			log.Printf("Couldn't query for movies released in %v. Here's why: %v\n", userId, err)
		} else {
			err = attributevalue.UnmarshalListOfMaps(response.Items, &movies)
			if err != nil {
				log.Printf("Couldn't unmarshal query response. Here's why: %v\n", err)
			}
		}
	}

	if len(movies) == 0 {
		return ServerPush{}, fmt.Errorf("db: no messages found for uid (%v) and messageId (%v)", userId, mid)
	}

	return movies[0], err
}

func (db DynamoDbRepository) deleteMessageById(ctx context.Context, mid string, uid string) error {
	_, err := db.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(DDB_TABLE_USER_MESSAGES), Key: map[string]types.AttributeValue{
			"id":     &types.AttributeValueMemberS{Value: mid},
			"userId": &types.AttributeValueMemberS{Value: uid},
		},
	})
	if err != nil {
		log.Printf("Couldn't delete %v from the table. Here's why: %v\n", mid, err)
	}
	return err
}

func createDeviceTokenTable(ctx context.Context, d *dynamodb.Client) (*types.TableDescription, error) {
	if tableExists(d, DDB_TABLE_DEVICE_TOKEN) {
		log.Printf("table=%v already exists\n", DDB_TABLE_DEVICE_TOKEN)
		return nil, nil
	}
	var tableDesc *types.TableDescription
	table, err := d.CreateTable(ctx, &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{{
			AttributeName: aws.String("token"),
			AttributeType: types.ScalarAttributeTypeS,
		}, {
			AttributeName: aws.String("userId"),
			AttributeType: types.ScalarAttributeTypeS,
		}},
		KeySchema: []types.KeySchemaElement{{
			AttributeName: aws.String("userId"),
			KeyType:       types.KeyTypeHash,
		}, {
			AttributeName: aws.String("token"),
			KeyType:       types.KeyTypeRange,
		}},
		TableName:   aws.String(DDB_TABLE_DEVICE_TOKEN),
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		log.Printf("Couldn't create table %v. Here's why: %v\n", DDB_TABLE_DEVICE_TOKEN, err)
	} else {
		waiter := dynamodb.NewTableExistsWaiter(d)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(DDB_TABLE_DEVICE_TOKEN)}, 5*time.Minute)
		if err != nil {
			log.Printf("Wait for table exists failed. Here's why: %v\n", err)
		}
		tableDesc = table.TableDescription
	}
	return tableDesc, err
}

func (db DynamoDbRepository) addDeviceToken(ctx context.Context, token AddTokenModel) error {
	item, err := attributevalue.MarshalMap(token)
	if err != nil {
		panic(err)
	}
	_, err = db.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(DDB_TABLE_DEVICE_TOKEN), Item: item,
	})
	if err != nil {
		log.Printf("Couldn't add device token to table. Here's why: %v\n", err)
	}
	return err
}

func (db DynamoDbRepository) getDeviceTokens(ctx context.Context, userId string) ([]AddTokenModel, error) {
	var err error
	var response *dynamodb.QueryOutput
	var movies []AddTokenModel
	keyEx := expression.Key("userId").Equal(expression.Value(userId))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("Couldn't build epxression for query. Here's why: %v\n", err)
	} else {
		response, err = db.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(DDB_TABLE_DEVICE_TOKEN),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			KeyConditionExpression:    expr.KeyCondition(),
		})
		if err != nil {
			log.Printf("Couldn't query for movies released in %v. Here's why: %v\n", userId, err)
		} else {
			err = attributevalue.UnmarshalListOfMaps(response.Items, &movies)
			if err != nil {
				log.Printf("Couldn't unmarshal query response. Here's why: %v\n", err)
			}
		}
	}
	return movies, err
}

func createTaskTable(ctx context.Context, d *dynamodb.Client) (*types.TableDescription, error) {
	if tableExists(d, DDB_TABLE_TASK) {
		log.Printf("table=%v already exists\n", DDB_TABLE_TASK)
		return nil, nil
	}
	var tableDesc *types.TableDescription
	table, err := d.CreateTable(ctx, &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{{
			AttributeName: aws.String("id"),
			AttributeType: types.ScalarAttributeTypeS,
		}},
		KeySchema: []types.KeySchemaElement{{
			AttributeName: aws.String("id"),
			KeyType:       types.KeyTypeHash,
		}},
		TableName:   aws.String(DDB_TABLE_TASK),
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		log.Printf("Couldn't create table %v. Here's why: %v\n", DDB_TABLE_TASK, err)
	} else {
		waiter := dynamodb.NewTableExistsWaiter(d)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(DDB_TABLE_TASK)}, 5*time.Minute)
		if err != nil {
			log.Printf("Wait for table exists failed. Here's why: %v\n", err)
		}
		tableDesc = table.TableDescription
	}
	return tableDesc, err
}

func (db DynamoDbRepository) addTask(ctx context.Context, task AddTaskModel) error {
	item, err := attributevalue.MarshalMap(task)
	if err != nil {
		panic(err)
	}
	_, err = db.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(DDB_TABLE_TASK), Item: item,
	})
	if err != nil {
		log.Printf("Couldn't add task to table. Here's why: %v\n", err)
	}
	return err
}

func (db DynamoDbRepository) getTaskById(ctx context.Context, taskId string) (AddTaskModel, error) {
	var err error
	var response *dynamodb.QueryOutput
	var movies []AddTaskModel
	keyEx := expression.Key("id").Equal(expression.Value(taskId))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("Couldn't build epxression for query. Here's why: %v\n", err)
	} else {
		response, err = db.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(DDB_TABLE_TASK),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			KeyConditionExpression:    expr.KeyCondition(),
		})
		if err != nil {
			log.Printf("Couldn't query for task with id %v. Here's why: %v\n", taskId, err)
		} else {
			err = attributevalue.UnmarshalListOfMaps(response.Items, &movies)
			if err != nil {
				log.Printf("Couldn't unmarshal query response. Here's why: %v\n", err)
			}
		}
	}
	return movies[0], err
}

func createChatGroupTable(ctx context.Context, d *dynamodb.Client) (*types.TableDescription, error) {
	if tableExists(d, DDB_TABLE_CHAT_GROUP) {
		log.Printf("table=%v already exists\n", DDB_TABLE_CHAT_GROUP)
		return nil, nil
	}
	var tableDesc *types.TableDescription
	table, err := d.CreateTable(ctx, &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{{
			AttributeName: aws.String("id"),
			AttributeType: types.ScalarAttributeTypeS,
		}},
		KeySchema: []types.KeySchemaElement{{
			AttributeName: aws.String("id"),
			KeyType:       types.KeyTypeHash,
		}},
		TableName:   aws.String(DDB_TABLE_CHAT_GROUP),
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		log.Printf("Couldn't create table %v. Here's why: %v\n", DDB_TABLE_CHAT_GROUP, err)
	} else {
		waiter := dynamodb.NewTableExistsWaiter(d)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(DDB_TABLE_CHAT_GROUP)}, 5*time.Minute)
		if err != nil {
			log.Printf("Wait for table exists failed. Here's why: %v\n", err)
		}
		tableDesc = table.TableDescription
	}
	return tableDesc, err
}

func (db DynamoDbRepository) addChatGroup(ctx context.Context, cg AddChatGroupModel) error {
	item, err := attributevalue.MarshalMap(cg)
	if err != nil {
		panic(err)
	}
	_, err = db.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(DDB_TABLE_CHAT_GROUP), Item: item,
	})
	if err != nil {
		log.Printf("Couldn't add chat group to table. Here's why: %v\n", err)
	}
	return err
}

func (db DynamoDbRepository) getChatGroupById(ctx context.Context, chatId string) (AddChatGroupModel, error) {
	var err error
	var response *dynamodb.QueryOutput
	var movies []AddChatGroupModel
	keyEx := expression.Key("id").Equal(expression.Value(chatId))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("Couldn't build epxression for query. Here's why: %v\n", err)
	} else {
		response, err = db.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(DDB_TABLE_CHAT_GROUP),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			KeyConditionExpression:    expr.KeyCondition(),
		})
		if err != nil {
			log.Printf("Couldn't query for chat group with id %v. Here's why: %v\n", chatId, err)
		} else {
			err = attributevalue.UnmarshalListOfMaps(response.Items, &movies)
			if err != nil {
				log.Printf("Couldn't unmarshal query response. Here's why: %v\n", err)
			}
		}
	}
	return movies[0], err
}

func createChatGroupMemberTable(ctx context.Context, d *dynamodb.Client) (*types.TableDescription, error) {
	if tableExists(d, DDB_TABLE_CHAT_GROUP_MEMBER) {
		log.Printf("table=%v already exists\n", DDB_TABLE_CHAT_GROUP_MEMBER)
		return nil, nil
	}
	var tableDesc *types.TableDescription
	table, err := d.CreateTable(ctx, &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{{
			AttributeName: aws.String("chatId"),
			AttributeType: types.ScalarAttributeTypeS,
		}, {
			AttributeName: aws.String("memberUserId"),
			AttributeType: types.ScalarAttributeTypeS,
		}},
		KeySchema: []types.KeySchemaElement{{
			AttributeName: aws.String("chatId"),
			KeyType:       types.KeyTypeHash,
		}, {
			AttributeName: aws.String("memberUserId"),
			KeyType:       types.KeyTypeRange,
		}},
		TableName:   aws.String(DDB_TABLE_CHAT_GROUP_MEMBER),
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		log.Printf("Couldn't create table %v. Here's why: %v\n", DDB_TABLE_CHAT_GROUP_MEMBER, err)
	} else {
		waiter := dynamodb.NewTableExistsWaiter(d)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(DDB_TABLE_CHAT_GROUP_MEMBER)}, 5*time.Minute)
		if err != nil {
			log.Printf("Wait for table exists failed. Here's why: %v\n", err)
		}
		tableDesc = table.TableDescription
	}
	return tableDesc, err
}

func (db DynamoDbRepository) addChatGroupMember(ctx context.Context, m AddChatGroupMemberModel) error {
	item, err := attributevalue.MarshalMap(m)
	if err != nil {
		panic(err)
	}
	_, err = db.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(DDB_TABLE_CHAT_GROUP_MEMBER), Item: item,
	})
	if err != nil {
		log.Printf("Couldn't add chat group member to table. Here's why: %v\n", err)
	}
	return err
}

func (db DynamoDbRepository) getChatGroupMembers(ctx context.Context, chatId string) ([]AddChatGroupMemberModel, error) {
	var err error
	var response *dynamodb.QueryOutput
	var movies []AddChatGroupMemberModel
	keyEx := expression.Key("chatId").Equal(expression.Value(chatId))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		log.Printf("Couldn't build epxression for query. Here's why: %v\n", err)
	} else {
		response, err = db.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(DDB_TABLE_CHAT_GROUP_MEMBER),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			KeyConditionExpression:    expr.KeyCondition(),
		})
		if err != nil {
			log.Printf("Couldn't query for chat group members in chat group (%v). Here's why: %v\n", chatId, err)
		} else {
			err = attributevalue.UnmarshalListOfMaps(response.Items, &movies)
			if err != nil {
				log.Printf("Couldn't unmarshal query response. Here's why: %v\n", err)
			}
		}
	}
	return movies, err
}
