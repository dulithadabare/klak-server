package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gin-gonic/gin"
	"github.com/segmentio/kafka-go"
	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"

	"context"

	"log"
	"os"
	"os/signal"
	"syscall"

	"fmt"

	firebase "firebase.google.com/go/v4"

	"firebase.google.com/go/v4/auth"
	"firebase.google.com/go/v4/db"
	"firebase.google.com/go/v4/messaging"

	"google.golang.org/api/option"
)

type Contacts struct {
	Data []string `json:"data"`
}

type AppUser struct {
	Uid         string `json:"uid" dynamodbav:"uid"`
	PhoneNumber string `json:"phoneNumber"`
	PublicKey   string `json:"publicKey"`
	FirstName   string `json:"firstName" dynamodbav:"firstName"`
	LastName    string `json:"lastName" dynamodbav:"lastName"`
}

// var appUsers = map[string]AppUser{
// 	"+16505553434": {Uid: "lEfTVmhpaaXRfH2RtF403nSEBSj1", PhoneNumber: "+16505553434"},
// 	"+16505553535": {Uid: "FRUDL020nuOJoVnjRz6ypsHCXth2", PhoneNumber: "+16505553535"},
// 	"+16505553636": {Uid: "sfHopUEFKoWU8qUVapVAWwfqN0i1", PhoneNumber: "+16505553636"},
// }

var firebaseDbClient *db.Client
var authClient *auth.Client
var cognitoJWTAuth *JwtAuth
var cloudMessagingClient *messaging.Client
var w *kafka.Writer
var writerCtx context.Context
var writerCtxCancel context.CancelFunc
var hub *Hub
var Version = "development"
var dynamoDbClient *dynamodb.Client
var dbService *DatabaseService
var dynamoDbRepository DynamoDbRepository
var apnsClient *apns2.Client
var notificationService *NotificationService
var presenceMap map[string]AddPresenceModel
var presenceSubscribers map[string][]string

type ContextKey string

const logPrefix ContextKey = "log_prefix"
const (
	uidKey = "userId"
)

// var User string
var BuildTime string = "notset"
var BuildCommit string = "n/a"
var BuildCommitTitle string = "n/a"
var BuildJobId string = "n/a"

type Configuration struct {
	Type string
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func openJsonFile() {
	file, error := os.Open("hamuwemu-app-firebase-adminsdk-serviceAccountKey.json")
	check(error)
	defer file.Close()
	decoder := json.NewDecoder(file)
	configuration := Configuration{}
	err := decoder.Decode(&configuration)
	if err != nil {
		println("error:", err)
	}
	println(configuration.Type)
}

func readConfigFileJson() ([]byte, error) {
	file, err := os.ReadFile("hamuwemu-app-firebase-adminsdk-serviceAccountKey.json")
	if err != nil {
		if file, err := os.ReadFile("/home/hamuwemu-app-firebase-adminsdk-serviceAccountKey.json"); err == nil {
			return file, err
		}
		return nil, err
	}

	return file, nil
}

func readAPNSAuthKeyFileJson() ([]byte, error) {
	file, err := os.ReadFile("AuthKey_S9DN84Q7KS.p8")
	if err != nil {
		if file, err := os.ReadFile("/home/AuthKey_S9DN84Q7KS.p8"); err == nil {
			return file, err
		}
		return nil, err
	}

	return file, nil
}

func configureDynamoDbClient(ctx context.Context) *dynamodb.Client {
	// Using the SDK's default configuration, loading additional config
	// and credentials values from the environment variables, shared
	// credentials, and shared configuration files
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("ap-south-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{URL: "http://localhost:8000"}, nil
		})),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID: "AKIAU6P5SAWNFBX7VFMX", SecretAccessKey: "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", SessionToken: "",
				Source: "Hard-coded credentials; values are irrelevant for local DynamoDB",
			},
		}),
	)

	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	// Using the Config value, create the DynamoDB client
	return dynamodb.NewFromConfig(cfg)

}

func configureFirebase() {
	// [START authenticate_with_admin_privileges]
	ctx := context.Background()
	conf := &firebase.Config{
		DatabaseURL: "https://hamuwemu-app-default-rtdb.asia-southeast1.firebasedatabase.app",
	}

	cfg, err := readConfigFileJson()
	if err != nil {
		log.Fatalln("Error initializing app:", err)
	}

	// Fetch the service account key JSON file contents
	// opt := option.WithCredentialsFile("hamuwemu-app-firebase-adminsdk-serviceAccountKey.json")
	opt := option.WithCredentialsJSON(cfg)

	// Initialize the app with a service account, granting admin privileges
	app, err := firebase.NewApp(ctx, conf, opt)
	if err != nil {
		log.Fatalln("Error initializing app:", err)
	}

	client, err := app.Database(ctx)
	if err != nil {
		log.Fatalln("Error initializing database client:", err)
	}

	firebaseDbClient = client
	// As an admin, the app has access to read and write all data, regradless of Security Rules
	ref := client.NewRef("restricted_access/secret_document")
	var data map[string]interface{}
	if err := ref.Get(ctx, &data); err != nil {
		log.Fatalln("Error reading from database:", err)
	}
	fmt.Println(data)
	// [END authenticate_with_admin_privileges]

	//Create auth client
	authClientTemp, err := app.Auth(ctx)
	if err != nil {
		log.Fatalf("error getting Auth client: %v\n", err)
	}

	authClient = authClientTemp

	cloudMessagingClientTemp, err := app.Messaging(ctx)
	if err != nil {
		log.Fatalf("error getting Auth client: %v\n", err)
	}

	cloudMessagingClient = cloudMessagingClientTemp

}

func configureAuthMiddleware() *JwtAuth {
	auth := NewAuth(&Config{
		CognitoRegion:     "ap-south-1",
		CognitoUserPoolID: "ap-south-1_Ve673ueYP",
	})
	err := auth.CacheJWK()

	if err != nil {
		log.Fatal("Failed to get JWKS:", err)
	}

	return auth
}

func configureKafka() {
	w = kafka.NewWriter(kafka.WriterConfig{
		Brokers:   []string{broker1Address, broker2Address, broker3Address},
		Topic:     topic,
		BatchSize: 1,
		Balancer: &kafka.Murmur2Balancer{
			Consistent: true,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	writerCtx = ctx
	writerCtxCancel = cancel
}

func configAPNSClient() *apns2.Client {
	authKeyFile, err := readAPNSAuthKeyFileJson()
	if err != nil {
		log.Fatal("token error:", err)
	}

	authKey, err := token.AuthKeyFromBytes(authKeyFile)
	if err != nil {
		log.Fatal("token error:", err)
	}

	token := &token.Token{
		AuthKey: authKey,
		KeyID:   "S9DN84Q7KS",
		TeamID:  "J8733ZYUN5",
	}

	// notification := &apns2.Notification{}
	// notification.DeviceToken = *deviceToken
	// notification.Topic = *topic
	// notification.Payload = []byte(`{
	// 		"aps" : {
	// 			"alert" : "Hello!"
	// 		}
	// 	}
	// `)

	return apns2.NewTokenClient(token).Production()
	// res, err := client.Push(notification)

	// if err != nil {
	// 	log.Fatal("Error:", err)
	// }
}

func main() {
	fmt.Println("Version:\t", Version)
	fmt.Println("build.Time:\t", BuildTime)
	fmt.Println("build.Commit:\t", BuildCommit)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan bool, 1)

	ctx, cancel := context.WithCancel(context.Background())

	hub = &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan bool, 1),
	}

	go hub.run(ctx)

	dynamoDbClient = configureDynamoDbClient(ctx)
	apnsClient = configAPNSClient()
	dbService = &DatabaseService{
		dynamoDbRespository: &DynamoDbRepository{
			client: dynamoDbClient,
		},
	}
	notificationService = &NotificationService{
		apnsClient: apnsClient,
	}
	presenceMap = make(map[string]AddPresenceModel)
	presenceSubscribers = make(map[string][]string)

	createUserTable(ctx, dynamoDbClient)
	createMessageTable(ctx, dynamoDbClient)
	createDeviceTokenTable(ctx, dynamoDbClient)
	createTaskTable(ctx, dynamoDbClient)
	createChatGroupTable(ctx, dynamoDbClient)
	createChatGroupMemberTable(ctx, dynamoDbClient)

	// Build the request with its input parameters
	resp, err := dynamoDbClient.ListTables(ctx, &dynamodb.ListTablesInput{
		Limit: aws.Int32(5),
	})
	if err != nil {
		log.Fatalf("failed to list tables, %v", err)
	}

	fmt.Println("Tables:")
	for _, tableName := range resp.TableNames {
		fmt.Println(tableName)
	}

	configureFirebase()
	cognitoJWTAuth = configureAuthMiddleware()

	router := gin.New()

	// Global middleware
	// Logger middleware will write the logs to gin.DefaultWriter even if you set with GIN_MODE=release.
	// By default gin.DefaultWriter = os.Stdout
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health"},
	}))

	// Recovery middleware recovers from any panics and writes a 500 if there was one.
	router.Use(gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
		c.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/version", func(c *gin.Context) {
		c.IndentedJSON(http.StatusOK, gin.H{
			"version":          Version,
			"buildCommit":      BuildCommit,
			"buildCommitTitle": BuildCommitTitle,
			"buildJobId":       BuildJobId,
			"buildTime":        BuildTime,
		})
	})
	router.GET("/ping", func(c *gin.Context) {
		c.IndentedJSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// router.GET("/users/:userId/messages/:messageId", getMessageByUserId)
	router.POST("/users/:userId/messages/:messageId/ack", ackMessage)

	// Authorization group
	// authorized := r.Group("/", AuthRequired())
	// exactly the same as:
	authorized := router.Group("/")
	// per group middleware! in this case we use the custom created
	// AuthRequired() middleware just in the "authorized" group.
	authorized.Use(CognitoJWTTokenAuthMiddleware())
	{
		authorized.POST("/users", addUser)
		authorized.POST("/sync", syncContacts)
		authorized.GET("/chatIds", getChatIds)
		authorized.POST("/tokens", addToken)
		// authorized.POST("/groups", addGroup)
		authorized.GET("/messages", getMessages)
		authorized.POST("/ack", addDeliveredReceipt)
		authorized.POST("/read", addReadReceipt)
		authorized.PUT("/threads/:threadId/title", updateThreadTitle)
		authorized.DELETE("/accounts", deleteAccount)
		// authorized.POST("/push", handlePush)
		// authorized.POST("/ack", addDeliveredReceipt)
		authorized.POST("/workspaces", addDemoWorkspace)
		authorized.GET("/workspaces/members", getWorkspaceMembers)
		authorized.GET("/users", getUsers)
		authorized.GET("/users/:userId", getUserById)
		authorized.GET("/presence/:peerId", getUserPresenceById)
		authorized.POST("/tasks", addTask)
		authorized.POST("/groups", addChatGroup)
		authorized.POST("/groups/:groupId/members", addChatGroupMember)
		authorized.POST("/chats", addChat)
		authorized.GET("/ws", func(c *gin.Context) {
			userUid := c.MustGet(uidKey).(string)
			serveWs(ctx, hub, c.Writer, c.Request, userUid, 0)
			fmt.Println("Listening for events from ", userUid)
		})
	}

	// router.Run("localhost:8080")

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	// http.ListenAndServeTLS()

	fmt.Println("Listeneing on port :8080 ")

	go func() {
		// service connections
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("listen: %s\n", err)
		}
	}()

	go func() {
		sig := <-sigs

		cancel()
		<-hub.done

		log.Println("Closing Writer")
		if err := w.Close(); err != nil {
			log.Fatal("failed to close writer:", err)
		}

		fmt.Println()
		fmt.Println(sig)
		done <- true
	}()

	<-done
	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.

	// srv.RegisterOnShutdown()

	shutDownCtx, cancelShutDownCtx := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutDownCtx()
	if err := srv.Shutdown(shutDownCtx); err != nil {
		log.Fatal("Server Shutdown:", err)
	}
	log.Println("Server exiting")
}
