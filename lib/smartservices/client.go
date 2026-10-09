/*
 * Copyright 2026 InfAI (CC SES)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package smartservices asks the smart-service-repository which smart services use a process model.
package smartservices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SENERGY-Platform/process-model-repository/lib/model"
)

const (
	// kind of the process models in GET /resource-usage/{kind}/{id}.
	kindProcessModels = "process-models"
	requestTimeout    = 5 * time.Second
	// The answer lists the names the caller may read; this only bounds a misbehaving server.
	maxBodyBytes = 4 << 20
)

type Client struct {
	url  string
	http *http.Client
}

func New(baseUrl string) *Client {
	return &Client{url: strings.TrimRight(baseUrl, "/"), http: &http.Client{Timeout: requestTimeout}}
}

// Usage returns the smart-service-repository's answer for the process model, asked with the
// caller's own token. Any failure, including a non-200 status, is an error: an unknown answer is
// not "unused".
func (c *Client) Usage(ctx context.Context, processModelId string, auth string) (usage model.ResourceUsage, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/resource-usage/"+kindProcessModels+"/"+url.PathEscape(processModelId), nil)
	if err != nil {
		return usage, err
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return usage, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return usage, fmt.Errorf("smart-service-repository answered %d", resp.StatusCode)
	}
	// Pointers, so that a 200 without a count is an error and not "zero releases".
	var answer struct {
		Releases  *int                `json:"releases"`
		Instances *int                `json:"instances"`
		Readable  []model.ResourceRef `json:"readable"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&answer); err != nil {
		return usage, fmt.Errorf("decode smart-service-repository answer: %w", err)
	}
	if answer.Releases == nil || answer.Instances == nil {
		return usage, errors.New("smart-service-repository answer lacks releases or instances")
	}
	usage.Releases = *answer.Releases
	usage.Instances = *answer.Instances
	usage.Readable = answer.Readable
	if usage.Readable == nil {
		usage.Readable = []model.ResourceRef{}
	}
	return usage, nil
}
