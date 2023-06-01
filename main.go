package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	"github.com/go-redis/redis"
)

const (
	kafkaTopic = "hashedTokens"
	redisKey   = "timeout"
)

type Message struct {
	Datetime string `json:"datetime"`
	Data     int    `json:"data"`
	Timeout  int    `json:"timeout"`
}

func gSend(producer sarama.SyncProducer, tickCh <-chan bool, wg *sync.WaitGroup) {
	defer wg.Done()
	messageCount := 0

	for range tickCh {
		message := Message{
			Datetime: time.Now().UTC().String(),
			Data:     messageCount,
			Timeout:  getRandomTimeout(),
		}

		jsonData, err := json.Marshal(message)
		if err != nil {
			log.Println("Failed to marshal JSON data:", err)
			continue
		}

		// Send message to Kafka
		_, _, err = producer.SendMessage(&sarama.ProducerMessage{
			Topic: kafkaTopic,
			Value: sarama.ByteEncoder(jsonData),
		})
		if err != nil {
			log.Println("Failed to send message to Kafka:", err)
		} else {
			log.Println("Sent message to Kafka:", string(jsonData))
			messageCount++
		}
	}
}

func gRecv(consumer sarama.Consumer, wg *sync.WaitGroup) {
	defer wg.Done()
	partitionConsumer, err := consumer.ConsumePartition(kafkaTopic, 0, sarama.OffsetOldest)
	if err != nil {
		log.Fatal("Failed to create partition consumer:", err)
	}
	defer partitionConsumer.Close()

	for message := range partitionConsumer.Messages() {
		var msg Message
		err := json.Unmarshal(message.Value, &msg)
		if err != nil {
			log.Println("Failed to unmarshal JSON data:", err)
			continue
		}

		log.Println("Received message from Kafka:", string(message.Value))
	}
}

func gTick(redisClient *redis.Client, tickCh chan<- bool, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		timeout := getRandomTimeout()
		err := redisClient.Set(redisKey, timeout, time.Duration(timeout)*time.Second).Err()
		if err != nil {
			log.Println("Failed to set Redis timeout:", err)
		} else {
			log.Println("Set Redis timeout:", timeout, "seconds")
		}

		// Send tick signal to gSend
		tickCh <- true

		time.Sleep(time.Duration(timeout) * time.Second)
	}
}

func main() {
	// Set up Kafka producer and consumer
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	producer, err := sarama.NewSyncProducer([]string{"localhost:9092"}, config)
	if err != nil {
		log.Fatal("Failed to set up Kafka producer:", err)
	}
	defer producer.Close()

	consumer, err := sarama.NewConsumer([]string{"localhost:9092"}, config)
	if err != nil {
		log.Fatal("Failed to set up Kafka consumer:", err)
	}
	defer consumer.Close()

	// Set up Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

	// WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	// Create tick channel
	tickCh := make(chan bool)

	// gSend: goroutine to send data into Kafka
	go gSend(producer, tickCh, &wg)

	// gRecv: goroutine to receive data from Kafka
	go gRecv(consumer, &wg)

	// gTick: goroutine to set random timeouts in Redis
	go gTick(redisClient, tickCh, &wg)

	// Wait for goroutines to finish
	wg.Wait()

	// Close tick channel after all goroutines have finished
	close(tickCh)
}

func getRandomTimeout() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(5) + 1
}
