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
	"math"
	"math/rand"
	"strconv"
	"time"
)

const dynamodbRegion = "eu-west-1"
const dynamodbTable = "TwitterBot"

const tgbotToken = "PUT_TOKEN_HERE"
const tgbotLogID = -170311435

const SecurityThreshold = 5

// Post structure for DynamoDB post table
type TwitterBot struct {
	ID         int     `json:"id"`
	ScreenName string  `json:"screenname"`
	Followers  []int64 `json:"followers"`
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
 // PUT CREDENTIALS HERE
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
		tglog(fmt.Sprintf("Twitter check para %v", user.ScreenName))
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

			followerIDs, _, _ := client.Followers.IDs(&twitter.FollowerIDParams{Count: 5000})
			arrayOfIDS := followerIDs.IDs
			for followerIDs.NextCursor != 0 {
				followerIDs, _, _ = client.Followers.IDs(&twitter.FollowerIDParams{Count: 5000, Cursor: followerIDs.NextCursor})
				arrayOfIDS = append(arrayOfIDS, followerIDs.IDs...)
			}

			if tw.ID != int(user.ID) {
				tw := TwitterBot{int(user.ID), user.ScreenName, arrayOfIDS}
				av, _ := dynamodbattribute.MarshalMap(tw)
				input := &dynamodb.PutItemInput{
					Item:      av,
					TableName: aws.String(dynamodbTable),
				}
				_, err = svc.PutItem(input)
			} else {
				diff := math.Abs(float64(len(arrayOfIDS) - len(tw.Followers)))
				if diff < SecurityThreshold {
					for _, follower := range arrayOfIDS {
						if !inArray(follower, tw.Followers) {
							tglog(fmt.Sprintf("New follower %v para %v\n", follower, user.ScreenName))
							if twitterAccount.followBack {
								client.Friendships.Create(&twitter.FriendshipCreateParams{UserID: follower})
							}
							if len(twitterAccount.greetings) > 0 {
								nfollower, _, _ := client.Users.Show(&twitter.UserShowParams{UserID: follower})
								rand.Seed(time.Now().UnixNano())
								client.Statuses.Update(fmt.Sprintf("@%v %v", nfollower.ScreenName, twitterAccount.greetings[rand.Intn(len(twitterAccount.greetings))]), &twitter.StatusUpdateParams{})
							}
						}
					}
					for _, follower := range tw.Followers {
						if !inArray(follower, arrayOfIDS) {
							tglog(fmt.Sprintf("New unfollower %v para %v\n", follower, user.ScreenName))
							if twitterAccount.unfollowback {
								client.Friendships.Destroy(&twitter.FriendshipDestroyParams{UserID: follower})
							}
						}
					}
					tw := TwitterBot{int(user.ID), user.ScreenName, arrayOfIDS}
					av, _ := dynamodbattribute.MarshalMap(tw)
					input := &dynamodb.PutItemInput{
						Item:      av,
						TableName: aws.String(dynamodbTable),
					}
					_, err = svc.PutItem(input)
				} else {
					tglog("Number of new followers/unfollowers above threshold. Skipping")
				}
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
