package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/segmentio/kafka-go"
)

// the topic and broker address are initialized as constants
const (
	topic          = "my-kafka-topic"
	broker1Address = "localhost:9093"
	broker2Address = "localhost:9094"
	broker3Address = "localhost:9095"
)

func produce(userUid string, message ServerPush) {
	// intialize the writer with the broker addresses, and the topic

	val, _ := json.Marshal(&message)

	// each kafka message has a key and value. The key is used
	// to decide which partition (and consequently, which broker)
	// the message gets published on

	// writeCtx is cancelled lastly. Ensuring the writer will be closed
	// first and all pending messages
	// are written
	err := w.WriteMessages(writerCtx, kafka.Message{
		Key: []byte(userUid),
		// create an arbitrary message payload for the value
		Value: val,
	})
	if err != nil {
		if err == io.ErrClosedPipe {
			log.Println("WriterClosed: true")
			return
		}
		if errors.Is(err, context.Canceled) {
			log.Println("WriterContextCancelled: true")
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Println("WriterContextDeadlineExceeded: true")
			return
		}
		// panic("could not write message " + err.Error())
		log.Fatal("failed to write messages:", err.Error())
	}

	// log a confirmation once the message is written
	fmt.Println("writes:", message.Id)
	// sleep for a second
	// time.Sleep(time.Second)
}

func consume(ctx context.Context, userUid string, offset int64, send chan<- kafka.Message, done chan<- bool) {
	// initialize a new reader with the brokers and topic
	// the groupID identifies the consumer and prevents
	// it from receiving duplicate messages
	idx := (murmur2([]byte(userUid)) & 0x7fffffff) % uint32(3)
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{broker1Address, broker2Address, broker3Address},
		Topic:     topic,
		Partition: int(idx),
	})
	r.SetOffset(offset)

	defer func() {
		if err := r.Close(); err != nil {
			log.Fatal("failed to close reader:", err)
		}
		done <- true
		close(send)
	}()

	for {
		// the `ReadMessage` method blocks until we receive the next event
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if err == io.EOF {
				log.Println("EOF: true")
				break
			}
			if errors.Is(err, context.Canceled) {
				log.Println("ContextCancelled: true")
				break
			}
			if errors.Is(err, context.DeadlineExceeded) {
				log.Println("ContextDeadlineExceeded: true")
				break
			}
			log.Fatal("could not read message " + err.Error())
			break
		}
		// after receiving the message, log its value
		fmt.Printf("message at topic/partition/offset %v/%v/%v: %s = %s\n", m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value))
		// fmt.Printf("message at offset %d: %s = %s\n", m.Offset, string(m.Key), string(m.Value))

		// send message to websocket
		select {
		case <-ctx.Done():
			return
		case send <- m:
			if err := r.CommitMessages(ctx, m); err != nil {
				log.Fatal("failed to commit messages:", err)
			}
		}
	}
}

// Go port of the Java library's murmur2 function.
// https://github.com/apache/kafka/blob/1.0/clients/src/main/java/org/apache/kafka/common/utils/Utils.java#L353
func murmur2(data []byte) uint32 {
	length := len(data)
	const (
		seed uint32 = 0x9747b28c
		// 'm' and 'r' are mixing constants generated offline.
		// They're not really 'magic', they just happen to work well.
		m = 0x5bd1e995
		r = 24
	)

	// Initialize the hash to a random value
	h := seed ^ uint32(length)
	length4 := length / 4

	for i := 0; i < length4; i++ {
		i4 := i * 4
		k := (uint32(data[i4+0]) & 0xff) + ((uint32(data[i4+1]) & 0xff) << 8) + ((uint32(data[i4+2]) & 0xff) << 16) + ((uint32(data[i4+3]) & 0xff) << 24)
		k *= m
		k ^= k >> r
		k *= m
		h *= m
		h ^= k
	}

	// Handle the last few bytes of the input array
	extra := length % 4
	if extra >= 3 {
		h ^= (uint32(data[(length & ^3)+2]) & 0xff) << 16
	}
	if extra >= 2 {
		h ^= (uint32(data[(length & ^3)+1]) & 0xff) << 8
	}
	if extra >= 1 {
		h ^= uint32(data[length & ^3]) & 0xff
		h *= m
	}

	h ^= h >> 13
	h *= m
	h ^= h >> 15

	return h
}
