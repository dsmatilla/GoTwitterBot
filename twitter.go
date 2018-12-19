package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"math/rand"
	"strconv"
	"time"
)

const dynamodbRegion = "eu-west-1"
const dynamodbTable = "TwitterBot"

const tgbotToken = "TOKEN"
const tgbotLogID = -170311435

// Post structure for DynamoDB post table
type TwitterBot struct {
	ID        int     `json:"id"`
	Followers []int64 `json:"followers"`
}

// Structure for credentials
type Credentials struct {
	consumerKey    string
	consumerSecret string
	token          string
	tokenSecret    string
	followBack     bool
	unfollowback   bool
	greetings      []string
}

var twitterAccounts = []Credentials{
	{
		"",
		"",
		"",
		"",
		true,
		true,
		[]string{
			"Hola!",
		},
	},
	{
		"",
		"",
		"",
		"",
		true,
		true,
		[]string{
			"Hola!",
		},
	},
}

func tglog(message string) {
	bot, _ := tgbotapi.NewBotAPI(tgbotToken)
	msg := tgbotapi.NewMessage(tgbotLogID, fmt.Sprint(message))
	bot.Send(msg)
}

func inArray(account int64, followers []int64) (bool) {
	for _, follower := range followers {
		if account == follower {
			return true
		}
	}
	return false
}

func process() {
	// New session to dynamoDB
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String(dynamodbRegion)},
	)
	svc := dynamodb.New(sess)

	// Walk twitter accounts
	for _, twitterAccount := range twitterAccounts {
		config := oauth1.NewConfig(twitterAccount.consumerKey, twitterAccount.consumerSecret)
		token := oauth1.NewToken(twitterAccount.token, twitterAccount.tokenSecret)
		httpClient := config.Client(oauth1.NoContext, token)
		client := twitter.NewClient(httpClient)

		user, _, err := client.Accounts.VerifyCredentials(&twitter.AccountVerifyParams{})
		if err == nil {
			// Check if account is already there, otherwise launch initial import
			result, _ := svc.GetItem(&dynamodb.GetItemInput{
				TableName: aws.String(dynamodbTable),
				Key: map[string]*dynamodb.AttributeValue{
					"id": {
						N: aws.String(strconv.FormatInt(user.ID, 10)),
					},
				},
			})

			tw := TwitterBot{}
			err = dynamodbattribute.UnmarshalMap(result.Item, &tw)

			followerIDs, _, _ := client.Followers.IDs(&twitter.FollowerIDParams{Count: 1000000})

			if tw.ID != int(user.ID) {
				tw := TwitterBot{int(user.ID), followerIDs.IDs}
				av, _ := dynamodbattribute.MarshalMap(tw)
				input := &dynamodb.PutItemInput{
					Item:      av,
					TableName: aws.String(dynamodbTable),
				}
				_, err = svc.PutItem(input)
			} else {
				if twitterAccount.followBack {
					for _, follower := range followerIDs.IDs {
						if !inArray(follower, tw.Followers) {
							tglog(fmt.Sprintf("New follower %v para %v\n", follower, user.ScreenName))
							client.Friendships.Create(&twitter.FriendshipCreateParams{UserID: follower})
							if len(twitterAccount.greetings) > 0 {
								nfollower, _, _ := client.Users.Show(&twitter.UserShowParams{UserID: follower})
								rand.Seed(time.Now().UnixNano())
								client.Statuses.Update(fmt.Sprintf("@%v %v", nfollower.ScreenName, twitterAccount.greetings[rand.Intn(len(twitterAccount.greetings))]), &twitter.StatusUpdateParams{})
							}
						}
					}
				}
				if twitterAccount.unfollowback {
					for _, follower := range tw.Followers {
						if !inArray(follower, followerIDs.IDs) {
							tglog(fmt.Sprintf("New unfollower %v para %v\n", follower, user.ScreenName))
							client.Friendships.Destroy(&twitter.FriendshipDestroyParams{UserID: follower})
						}
					}
				}
				tw := TwitterBot{int(user.ID), followerIDs.IDs}
				av, _ := dynamodbattribute.MarshalMap(tw)
				input := &dynamodb.PutItemInput{
					Item:      av,
					TableName: aws.String(dynamodbTable),
				}
				_, err = svc.PutItem(input)
			}
		}
	}
}

func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	process()
	return events.APIGatewayProxyResponse{Body: request.Body, StatusCode: 200}, nil
}

func main() {
	lambda.Start(handleRequest)
}
