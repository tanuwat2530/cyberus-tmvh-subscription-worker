package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Start background worker in a goroutine
	go backgroundWorker()
	// Keep main running so the program doesn't exit
	select {} // This blocks forever
}

// This is the background job that runs forever
func backgroundWorker() {
	redisConnection := os.Getenv("BN_REDIS_URL")
	dbConnection := os.Getenv("BN_DB_URL")
	var ctx = context.Background()
	var WAIT_INTERVAL = (17 * time.Second)
	var cursor uint64 = 0
	var matchPattern = "tmvh-subscription-callback-api:*" // Pattern to match keys
	var count = int64(100)                                // Limit to 100 keys per scan

	fmt.Println("##### TMVH SUBSCRIPTION WORKER RUNNING #####")
	//config redis pool
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisConnection, // Change if needed
		Password: "",              // No password by default
		DB:       0,               // Default DB
		PoolSize: 100,             //Connection pools
	})

	//config database pool
	db, errDatabase := gorm.Open(postgres.Open(dbConnection), &gorm.Config{})
	if errDatabase != nil {
		log.Fatal("Failed to connect to database:", errDatabase)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("Failed to get generic database object:", err)
	}
	// Set connection pool settings
	sqlDB.SetMaxOpenConns(100)                // Maximum number of open connections
	sqlDB.SetMaxIdleConns(10)                 // Maximum number of idle connections
	sqlDB.SetConnMaxLifetime(5 * time.Minute) // Connection max lifetime

	var wg sync.WaitGroup

	for {

		// Perform the scan with the match pattern and count
		keys, newCursor, err := rdb.Scan(ctx, cursor, matchPattern, count).Result()
		if err != nil {
			panic(err)
		}
		if len(keys) > 0 {
			//fmt.Printf("number of key : %d\n", len(keys))
			for i := 0; i < len(keys); i++ {
				//fmt.Printf("key[%d] : ", i)
				//fmt.Println(keys[i])

				//fmt.Println("Send Key to Worker : ", keys[i]) // Print only the first key
				// Example: Get the value of the key (assuming it's a string)
				valJson, err := rdb.Get(ctx, keys[i]).Result()
				if err != nil {
					log.Fatal("Error getting value : ", err)
				} else {
					// Start multiple goroutines (threads)
					wg.Add(1)
					go threadWorker(i, &wg, valJson, rdb, ctx, db)
				}

			}

		}

		//for { // for infinity loop
		// Process only the first key found (if any)
		// if len(keys) >= LIMIT_KEY {
		// 	for i := 0; i < LIMIT_KEY; i++ {
		// 		//fmt.Println("Send Key to Worker : ", keys[i]) // Print only the first key
		// 		// Example: Get the value of the key (assuming it's a string)
		// 		valJson, err := rdb.Get(ctx, keys[i]).Result()
		// 		if err != nil {
		// 			fmt.Println("Error getting value : ", err)

		// 		} else {
		// 			//fmt.Println("Value:", val)
		// 			// Start multiple goroutines (threads)
		// 			wg.Add(1)
		// 			go threadWorker(i, &wg, valJson, rdb, ctx, db)
		// 			rdb.Del(ctx, keys[i]).Result()
		// 			// Wait for all goroutines to finish
		// 		}
		// 	}
		// }

		// if len(keys) > 0 {
		// 	//fmt.Println("Send Key to Worker : ", keys[i]) // Print only the first key
		// 	// Example: Get the value of the key (assuming it's a string)
		// 	valJson, err := rdb.Get(ctx, keys[0]).Result()
		// 	if err != nil {
		// 		fmt.Println("Error getting value : ", err)

		// 	} else {
		// 		//fmt.Println("Value:", val)
		// 		// Start multiple goroutines (threads)
		// 		wg.Add(1)
		// 		go threadWorker(0, &wg, valJson, rdb, ctx, db)
		// 		rdb.Del(ctx, keys[0]).Result()
		// 		// Wait for all goroutines to finish
		// 	}
		// }

		// Update cursor for the next iteration
		cursor = newCursor
		// If the cursor is 0, then the scan is complete
		if cursor == 0 {
			fmt.Println("Wait for next scan")
			time.Sleep(WAIT_INTERVAL)
			//break
		}
		wg.Wait() // Block here until all goroutines call Done()
	}
	//}

}

// Function that simulates work for a thread
func threadWorker(id int, wg *sync.WaitGroup, jsonString string, rdb *redis.Client, ctx context.Context, db *gorm.DB) error {

	type SubscriptionData struct {
		Msisdn       string `json:"msisdn"`
		Shortcode    string `json:"short-code"`
		Operator     string `json:"operator"`
		Action       string `json:"action"`
		Code         string `json:"code"`
		Desc         string `json:"desc"`
		Timestamp    int    `json:"timestamp"`
		TranRef      string `json:"tran-ref"`
		RefId        string `json:"ref-id"`
		Media        string `json:"media"`
		Token        string `json:"token"`
		ReturnStatus string `json:"cyberus-return"`
	}

	//Table name on database
	type tmvh_subscription_logs struct {
		ID            string `gorm:"primaryKey"`
		Action        string `gorm:"column:action"`
		Code          string `gorm:"column:code"`
		CyberusReturn string `gorm:"column:cyberus_return"`
		Description   string `gorm:"column:description"`
		Media         string `gorm:"column:media"`
		Msisdn        string `gorm:"column:msisdn"`
		Operator      string `gorm:"column:operator"`
		RefID         string `gorm:"column:ref_id"`
		ShortCode     string `gorm:"column:short_code"`
		Timestamp     int64  `gorm:"column:timestamp"`
		Token         string `gorm:"column:token"`
		TranRef       string `gorm:"column:tran_ref"`
	}
	//	fmt.Printf("Worker No : %d\n start", id)

	// Convert struct to JSON string
	var subscriptionData SubscriptionData
	errSubscriptionData := json.Unmarshal([]byte(jsonString), &subscriptionData)
	if errSubscriptionData != nil {
		fmt.Println("JSON Marshal error : ", errSubscriptionData)
		return fmt.Errorf("JSON DECODE ERROR : " + errSubscriptionData.Error())
	}

	// // Print the data to the console
	//fmt.Println("##### Insert into Database #####")
	//fmt.Println("Msisdn : " + transactionData.Msisdn)
	// fmt.Println("Shortcode : " + transactionData.Shortcode)
	// fmt.Println("Operator  : " + transactionData.Operator)
	// fmt.Println("Action  : " + transactionData.Action)
	// fmt.Println("Code  : " + transactionData.Code)
	// fmt.Println("Desc  : " + transactionData.Desc)
	//fmt.Println("Timestamp  : " + strconv.FormatInt(int64(transactionData.Timestamp)))
	// fmt.Println("TranRef  : " + transactionData.TranRef)
	// fmt.Println("Action  : " + transactionData.Action)
	// fmt.Println("RefId  : " + transactionData.RefId)
	// fmt.Println("Media  : " + transactionData.Media)
	// fmt.Println("Token  : " + transactionData.Token)
	// fmt.Println("CyberusReturn  : " + transactionData.ReturnStatus)

	//defer wg.Done() // Mark this goroutine as done when it exits

	logEntry := tmvh_subscription_logs{
		ID:            subscriptionData.RefId,
		Action:        subscriptionData.Action,
		Code:          subscriptionData.Code,
		CyberusReturn: subscriptionData.ReturnStatus,
		Description:   subscriptionData.Desc,
		Media:         subscriptionData.Media,
		Msisdn:        subscriptionData.Msisdn,
		Operator:      subscriptionData.Operator,
		RefID:         subscriptionData.RefId,
		ShortCode:     subscriptionData.Shortcode,
		Timestamp:     int64(subscriptionData.Timestamp),
		Token:         subscriptionData.Token,
		TranRef:       subscriptionData.TranRef,
	}

	if errInsertDB := db.Create(&logEntry).Error; errInsertDB != nil {
		fmt.Println("ERROR INSERT : " + errInsertDB.Error())
		return fmt.Errorf(errInsertDB.Error())
	}

	redis_set_key := "tmvh-transaction-log-worker:" + subscriptionData.Media + ":" + subscriptionData.RefId
	ttl := 240 * time.Hour // expires in 10 day
	// Set key with TTL
	errSetRedis := rdb.Set(ctx, redis_set_key, jsonString, ttl).Err()
	if errSetRedis != nil {
		fmt.Println("Redis SET error:", errSetRedis)
		return fmt.Errorf("REDIS SET ERROR : " + errSetRedis.Error())
	}

	redis_del_key := "tmvh-subscription-callback-api:" + subscriptionData.Media + ":" + subscriptionData.RefId
	rdb.Del(ctx, redis_del_key).Result()

	wg.Done()
	fmt.Printf("Worker No : %d finished\n", id)
	return nil
}
