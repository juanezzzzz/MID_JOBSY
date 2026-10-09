package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"api_MID/models"
)

type sobre struct {
	success bool `json: "success"`
	Status int `json: "status"`
	Message string `json: "message"`
	Data json.RawMessage `json: "data"`
	Date json.RawMessage `json: "date"`
}

func