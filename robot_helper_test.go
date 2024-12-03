// Copyright (c) Huawei Technologies Co., Ltd. 2024. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"github.com/opensourceways/robot-framework-lib/client"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/stretchr/testify/mock"
)

type mockClient struct {
	mock.Mock
	successfulCreatePRComment                bool
	successfulDeletePRComment                bool
	successfulCheckCLASignature              bool
	successfulAddPRLabels                    bool
	successfulRemovePRLabels                 bool
	successfulCheckIfPRCreateEvent           bool
	successfulCheckIfPRSourceCodeUpdateEvent bool
	successfulGetPullRequestCommits          bool
	successfulGetPullRequestLabels           bool
	method                                   string
	commits                                  []client.PRCommit
	prComments                               []client.PRComment
	labels                                   []string
	CLAState                                 string
}

func (m *mockClient) CreatePRComment(org, repo, number, comment string) bool {
	m.method = "CreatePRComment"
	return m.successfulCreatePRComment
}

func (m *mockClient) DeletePRComment(org, repo, commentID string) bool {
	m.method = "DeletePRComment"
	return m.successfulDeletePRComment
}

func (m *mockClient) CheckCLASignature(urlStr string) (string, bool) {
	m.method = "CheckCLASignature"
	return m.CLAState, m.successfulCheckCLASignature
}

func (m *mockClient) AddPRLabels(org, repo, number string, labels []string) bool {
	m.method = "AddPRLabels"
	return m.successfulAddPRLabels
}

func (m *mockClient) RemovePRLabels(org, repo, number string, labels []string) bool {
	m.method = "RemovePRLabels"
	return m.successfulRemovePRLabels
}

func (m *mockClient) CheckIfPRCreateEvent(evt *client.GenericEvent) bool {
	m.method = "CheckIfPRCreateEvent"
	return m.successfulCheckIfPRCreateEvent
}

func (m *mockClient) CheckIfPRSourceCodeUpdateEvent(evt *client.GenericEvent) bool {
	m.method = "CheckIfPRSourceCodeUpdateEvent"
	return m.successfulCheckIfPRSourceCodeUpdateEvent
}

func (m *mockClient) GetPullRequestCommits(org, repo, number string) ([]client.PRCommit, bool) {
	m.method = "GetPullRequestCommits"
	return m.commits, m.successfulGetPullRequestCommits
}

func (m *mockClient) GetPullRequestLabels(org, repo, number string) ([]string, bool) {
	m.method = "GetPullRequestLabels"
	return m.labels, m.successfulGetPullRequestLabels
}

func (m *mockClient) ListPullRequestComments(org, repo, number string) ([]client.PRComment, bool) {
	m.method = "GetPullRequestLabels"
	return m.prComments, m.successfulGetPullRequestLabels
}

const (
	org       = "org1"
	repo      = "repo1"
	number    = "1"
	commenter = "commenter1"
	label     = "label1"
)

func TestRemoveCLASignGuideComment(t *testing.T) {

	mc := new(mockClient)
	bot := &robot{cli: mc, cnf: &configuration{
		PlaceholderCLASignGuideTitle: "#123",
	}}

	cli, ok := bot.cli.(*mockClient)
	assert.Equal(t, true, ok)
	case1 := "ListPullRequestComments"
	cli.method = case1
	// No comments to remove
	bot.removeCLASignGuideComment(org, repo, number)
	assert.Equal(t, case1, cli.method)

}