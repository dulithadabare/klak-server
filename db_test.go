package main

import (
	"context"
	"testing"

	"github.com/kjk/betterguid"
)

func TestAddUser(t *testing.T) {
	client := configureDynamoDbClient(context.TODO())
	service := &DatabaseService{
		dynamoDbRespository: &DynamoDbRepository{
			client: client,
		},
	}

	user := AddUserModel{
		Uid:         "Test",
		PhoneNumber: "2",
		DisplayName: "3",
		PublicKey:   "4",
	}

	err := service.addUser(context.TODO(), user)
	if err != nil {
		t.Error(err)
	}

}

func TestAddMessage(t *testing.T) {
	client := configureDynamoDbClient(context.TODO())
	service := &DatabaseService{
		dynamoDbRespository: &DynamoDbRepository{
			client: client,
		},
	}

	messageReceipt := MessageReceiptModel{
		Type:      Sent,
		MessageId: "1",
		AppUser:   "1",
	}

	message := ServerPush{
		Id:     betterguid.New(),
		UserId: "1",
		Type:   ServerPushMessageReceipt,
		Data:   messageReceipt,
	}

	err := service.addMessage(context.TODO(), message)
	if err != nil {
		t.Error(err)
	}

}

func TestGetMessages(t *testing.T) {
	client := configureDynamoDbClient(context.TODO())
	service := &DatabaseService{
		dynamoDbRespository: &DynamoDbRepository{
			client: client,
		},
	}

	messages, err := service.getMessages(context.TODO(), "1")
	if err != nil {
		t.Error(err)
	}

	if len(messages) != 1 {
		t.Fail()
	}
}
