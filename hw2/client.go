package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/segmentio/kafka-go"
)

var brokers = []string{"localhost:9094", "localhost:9194", "localhost:9294"}
var topic = "my-topic"

type Event struct {
	Type string `json:"type"`
}

type PurchaseEvent struct {
	Event
	Amount int `json:"amount"`
}

type ClickEvent struct {
	Event
	Page int `json:"page"`
}

var realTimeAmount = 0
var realTimeViews = map[int]int{}
var lastTimestamp = time.Now().UTC()

func produce(event string) {
	ctx := context.Background()

	var acks int
	switch event {
	case "purchase":
		acks = -1
	case "click":
		acks = 1
	default:
		log.Fatalf("Unexpected type '%s'.", event)
	}

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:      brokers,
		Topic:        topic,
		BatchTimeout: 100 * time.Millisecond,
		RequiredAcks: acks,
	})
	defer writer.Close()

	for {
		var messageBytes []byte
		switch event {
		case "purchase":
			data, err := json.Marshal(PurchaseEvent{Event: Event{Type: "purchase"}, Amount: rand.Intn(10_000)})
			if err != nil {
				log.Fatalf("Produce error for type '%s': '%s'.\n", event, err)
			}
			messageBytes = data
		case "click":
			data, err := json.Marshal(ClickEvent{Event: Event{Type: "click"}, Page: rand.Intn(3)})
			if err != nil {
				log.Fatalf("Produce error for type '%s': '%s'.\n", event, err)
			}
			messageBytes = data
		default:
			log.Fatalf("Unexpected type '%s'.", event)
			return
		}

		err := writer.WriteMessages(ctx, kafka.Message{
			Value: messageBytes,
		})

		json := string(messageBytes)
		if err != nil {
			fmt.Printf("Produce error for type '%s', json: '%s': '%s'.\n", event, json, err)
			time.Sleep(1 * time.Second)
		} else {
			fmt.Printf("Produced for type '%s', json: '%s'.\n", event, json)
		}
	}
}

func consume(event string, groupID string) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
	})
	defer reader.Close()

	for {
		msg, err := reader.ReadMessage(context.Background())
		if err != nil {
			fmt.Printf("For group '%s', consume error: '%s'.\n", groupID, err)
			time.Sleep(1 * time.Second)
			continue
		}

		fmt.Printf("For group '%s', has read: '%s'.\n", groupID, string(msg.Value))
		eventMsg := Event{}
		err = json.Unmarshal(msg.Value, &eventMsg)
		if err != nil {
			fmt.Printf("For group '%s', parsing error: '%s'.\n", groupID, err)
			continue
		} else {
			if event != eventMsg.Type {
				fmt.Printf("For group '%s', skipped: '%s'.\n", groupID, eventMsg.Type)
				continue
			}
			switch event {
			case "purchase":
				purchaseEvent := PurchaseEvent{}
				err = json.Unmarshal(msg.Value, &purchaseEvent)
				if err != nil {
					fmt.Printf("For group '%s', parsing error: '%s'.\n", groupID, err)
				}
				consumePurchase(purchaseEvent)
			case "click":
				clickEvent := ClickEvent{}
				err = json.Unmarshal(msg.Value, &clickEvent)
				if err != nil {
					fmt.Printf("For group '%s', parsing error: '%s'.\n", groupID, err)
				}
				consumeClick(clickEvent)
			default:
				log.Fatalf("For group '%s', unexpected type: '%s'.\n", groupID, event)
			}
		}

		commit(reader, msg, groupID)
	}
}

func consumeClick(clickEvent ClickEvent) {
	resetIfNeed()
	realTimeViews[clickEvent.Page] = realTimeViews[clickEvent.Page] + 1
	fmt.Printf("Updated views for page '%d': '%d'", clickEvent.Page, realTimeViews[clickEvent.Page])
	time.Sleep(50 * time.Millisecond)

}

func consumePurchase(purchaseEvent PurchaseEvent) {
	resetIfNeed()
	realTimeAmount += purchaseEvent.Amount
	fmt.Printf("Updated amount: sum '%d' with new value: '%d'.\n", realTimeAmount, purchaseEvent.Amount)
	time.Sleep(400 * time.Millisecond)
}

func resetIfNeed() {
	if time.Now().UTC().Sub(lastTimestamp) > 15*time.Second {
		fmt.Println("Timestamp reset.")
		realTimeViews = make(map[int]int)
		realTimeAmount = 0
		lastTimestamp = time.Now().UTC()
	}
}

func commit(reader *kafka.Reader, msg kafka.Message, groupID string) {
	err := reader.CommitMessages(context.Background(), msg)
	if err != nil {
		fmt.Printf("For group '%s', consume commit error: '%s'.\n", groupID, err)
		time.Sleep(1 * time.Second)
	}
}

func main() {
	eventPtr := flag.String("event", "", "Either 'purchase' or 'click'.")
	producerPtr := flag.Bool("producer", false, "Should client be producer.")
	consumerPtr := flag.Bool("consumer", false, "Should client be consumer.")
	groupPtr := flag.String("group", "", "GroupID for consumer.")

	flag.Parse()

	if !*producerPtr && !*consumerPtr || *producerPtr && *consumerPtr {
		log.Fatal("Client must be either producer or consumer. Use --help flag.")
	}

	if *producerPtr && (*groupPtr != "" || *eventPtr == "") {
		log.Fatal("Set correct parameters for producer (producer can't use 'group' and must use 'event'). Use --help flag.")
	}

	if *consumerPtr && (*groupPtr == "" || *eventPtr == "") {
		log.Fatal("Set correct parameters for consumer (consumer must use 'event' and 'group'). Use --help flag.")
	}

	if *producerPtr {
		produce(*eventPtr)
	} else if *consumerPtr {
		consume(*eventPtr, *groupPtr)
	} else {
		log.Fatal("Unexpected flag processing.")
	}
}
