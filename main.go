package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/mitchellh/hashstructure/v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

// TODO support more request options

type Request struct {
	URL string `json:"url"`
}

type SavedResponse struct {
	Body        string `json:"string"`
	ContentType string `json:"ContentType"`
}

type Handler struct {
	Expiry time.Duration
	Redis  *redis.Client
}

func (h *Handler) request(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(fmt.Errorf("error reading supplie request body: %w", err), w)
		return
	}

	request := Request{}
	err = json.Unmarshal(body, &request)
	if err != nil {
		writeError(fmt.Errorf("error unmarshalling supplied request body: %w", err), w)
		return
	}

	hash, err := hashstructure.Hash(request, hashstructure.FormatV2, nil)
	if err != nil {
		writeError(fmt.Errorf("error hashing supplied request: %w", err), w)
		return
	}

	savedResponse, ok, err := h.getCache(hash)

	if ok {
		log.Printf("retreiving request from cache: %v\n", hashToStr(hash))
		writeResponse(savedResponse, w)
		return
	}

	if err != nil {
		log.Printf("error retrieving cache: %v\n", err)
	}

	log.Printf("performing request: %v\n", hashToStr(hash))

	savedResponse, err = makeRequest(request)
	if err != nil {
		writeError(fmt.Errorf("error making specified request: %w", err), w)
		return
	}

	err = h.setCache(hash, savedResponse)
	if err != nil {
		log.Printf("error setting cache: %v\n", err)
	}

	writeResponse(savedResponse, w)
}

func makeRequest(request Request) (SavedResponse, error) {
	resp, err := http.Get(request.URL)
	if err != nil {
		return SavedResponse{}, fmt.Errorf("error making request: %w", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return SavedResponse{}, fmt.Errorf("error reading request response: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	savedResponse := SavedResponse{
		Body:        string(body),
		ContentType: contentType,
	}

	return savedResponse, nil
}

func writeResponse(response SavedResponse, w http.ResponseWriter) {
	w.Header().Set("Content-Type", response.ContentType)
	_, err := w.Write([]byte(response.Body))
	if err != nil {
		writeError(fmt.Errorf("error writing response: %w", err), w)
		return
	}
}

func writeError(err error, w http.ResponseWriter) {
	// TODO improve this
	log.Printf("error occurred processing request: %v", err)
	w.WriteHeader(500)
}

func (h *Handler) getCache(hash uint64) (SavedResponse, bool, error) {
	ctx := context.Background()

	val, err := h.Redis.Get(ctx, hashToStr(hash)).Result()
	if err == redis.Nil {
		return SavedResponse{}, false, nil
	} else if err != nil {
		return SavedResponse{}, false, fmt.Errorf("error retrieving cache: %w", err)
	}

	savedResponse := SavedResponse{}
	err = json.Unmarshal([]byte(val), &savedResponse)
	if err != nil {
		return SavedResponse{}, false, fmt.Errorf("error unmarshalling cache: %w", err)
	}

	return savedResponse, true, nil
}

func (h *Handler) setCache(hash uint64, response SavedResponse) error {
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("error marshaling response as JSON: %w", err)
	}

	ctx := context.Background()

	err = h.Redis.Set(ctx, hashToStr(hash), string(data), h.Expiry).Err()
	if err != nil {
		return fmt.Errorf("error caching response: %w", err)
	}

	return nil
}

func hashToStr(hash uint64) string {
	return strconv.FormatUint(hash, 16)
}

func handleRequests(expiry time.Duration, redisClient *redis.Client) {
	h := Handler{
		Expiry: expiry,
		Redis:  redisClient,
	}
	http.HandleFunc("/", h.request)
	log.Fatal(http.ListenAndServe(":8000", nil))
}

func main() {
	expiry, err := time.ParseDuration("1m")
	if err != nil {
		panic(err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		panic(err)
	}

	handleRequests(expiry, rdb)
}
