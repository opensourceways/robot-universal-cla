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
	successfulListPullRequestComments        bool
	successfulCheckPermission                bool
	permission                               bool
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
	m.method = "ListPullRequestComments"
	return m.prComments, m.successfulListPullRequestComments
}

func (m *mockClient) CheckPermission(org, repo, username string) (bool, bool) {
	m.method = "CheckPermission"
	return m.permission, m.successfulCheckPermission
}

const (
	org       = "org1"
	repo      = "repo1"
	number    = "1"
	commenter = "commenter1"
	labelYes  = "label-yes"
	labelNo   = "label-no"
)

func TestRemoveCLASignGuideComment(t *testing.T) {

	mc := new(mockClient)
	bot := &robot{cli: mc, cnf: &configuration{
		PlaceholderCLASignGuideTitle: "#123",
		PlaceholderCLASignPassTitle:  "#456",
	}}
	cli, ok := bot.cli.(*mockClient)
	assert.Equal(t, true, ok)

	case1 := "ListPullRequestComments"
	cli.method = ""
	// get comments failed
	bot.removeCLASignGuideComment(org, repo, number)
	execMethod1 := cli.method
	assert.Equal(t, case1, execMethod1)

	cli.method = ""
	cli.successfulListPullRequestComments = true
	// getting comments to remove
	bot.removeCLASignGuideComment(org, repo, number)
	execMethod2 := cli.method
	assert.Equal(t, case1, execMethod2)

	cli.method = ""
	cli.prComments = []client.PRComment{
		{
			"123132",
			"111123",
		},
	}
	bot.cnf.PlaceholderCLASignGuideTitle = "222"
	// not found CLA sign guide comment
	bot.removeCLASignGuideComment(org, repo, number)
	execMethod3 := cli.method
	assert.Equal(t, case1, execMethod3)

	case4 := "DeletePRComment"
	cli.method = ""
	bot.cnf.PlaceholderCLASignGuideTitle = "111"
	// delete the CLA sign guide comment
	bot.removeCLASignGuideComment(org, repo, number)
	execMethod4 := cli.method
	assert.Equal(t, case4, execMethod4)
}

func TestWaitCLASignature(t *testing.T) {
	mc := new(mockClient)
	bot := &robot{cli: mc, cnf: &configuration{
		CommentSomeNeedSign:      "%s, 24, %s",
		PlaceholderCommitter:     "ddd",
		CommentUpdateLabelFailed: "14",
	}}
	cli, ok := bot.cli.(*mockClient)
	assert.Equal(t, true, ok)

	repoCnf := &repoConfig{
		CLALabelYes: labelYes,
		CLALabelNo:  labelNo,
	}

	case1 := "unsigned users is empty"
	cli.method = case1
	bot.waitCLASignature(org, repo, number, []string{}, []string{labelYes}, repoCnf)
	execMethod1 := cli.method
	assert.Equal(t, case1, execMethod1)

	case2 := "CreatePRComment"
	cli.method = ""
	// PR labels contains CLA failed label
	bot.waitCLASignature(org, repo, number, []string{"user1"}, []string{labelNo}, repoCnf)
	execMethod2 := cli.method
	assert.Equal(t, case2, execMethod2)

	case3 := "CreatePRComment"
	cli.method = ""
	cli.successfulAddPRLabels = true
	// remove CLA success label, and add CLA failed label
	bot.waitCLASignature(org, repo, number, []string{"user1"}, []string{labelYes}, repoCnf)
	execMethod3 := cli.method
	assert.Equal(t, case3, execMethod3)
}

func TestPassCLASignature(t *testing.T) {
	mc := new(mockClient)
	bot := &robot{cli: mc, cnf: &configuration{
		CommentAllSigned:         "%s, 99, %s",
		PlaceholderCommitter:     "aaa",
		CommentUpdateLabelFailed: "53",
	}}
	cli, ok := bot.cli.(*mockClient)
	assert.Equal(t, true, ok)

	repoCnf := &repoConfig{
		CLALabelYes: labelYes,
		CLALabelNo:  labelNo,
	}

	case1 := "CreatePRComment"
	cli.method = ""
	// PR labels contains CLA failed label and CLA success label
	bot.passCLASignature(org, repo, number, []string{"user2"}, []string{labelYes, labelNo}, repoCnf)
	execMethod1 := cli.method
	assert.Equal(t, case1, execMethod1)

	case2 := "CreatePRComment"
	cli.method = ""
	cli.successfulAddPRLabels = true
	// PR labels is empty
	bot.passCLASignature(org, repo, number, []string{"user3"}, []string{}, repoCnf)
	execMethod2 := cli.method
	assert.Equal(t, case2, execMethod2)

}

func TestListContributorNameAndEmail(t *testing.T) {
	mc := new(mockClient)
	bot := &robot{cli: mc, cnf: &configuration{}}
	repoCnf := &repoConfig{}

	var commits []client.PRCommit
	// PR commits is empty
	users, emails := bot.ListContributorNameAndEmail(commits, repoCnf)
	assert.Equal(t, []string{}, users)
	assert.Equal(t, []string{}, emails)

	commits1 := []client.PRCommit{
		{
			"u1",
			"e1",
			"u2",
			"e2",
		},
		{
			"u1",
			"e1",
			"u2",
			"e2",
		},
	}
	// use author info
	users1, emails1 := bot.ListContributorNameAndEmail(commits1, repoCnf)
	assert.Equal(t, true, len(users1) == 1 && len(emails1) == 1)
	assert.Equal(t, "u1", users1[0])
	assert.Equal(t, "e1", emails1[0])

	repoCnf.CheckByCommitter = true
	// use committer info
	users2, emails2 := bot.ListContributorNameAndEmail(commits1, repoCnf)
	assert.Equal(t, true, len(users2) == 1 && len(emails2) == 1)
	assert.Equal(t, "u2", users2[0])
	assert.Equal(t, "e2", emails2[0])
}

func TestCheckCLASignResult(t *testing.T) {
	mc := new(mockClient)
	bot := &robot{cli: mc, cnf: &configuration{}}
	cli, ok := bot.cli.(*mockClient)
	assert.Equal(t, true, ok)
	repoCnf := &repoConfig{
		LitePRCommitter: litePRCommiter{
			"e0",
			"u0",
		},
	}

	var commits []client.PRCommit
	// PR commits is empty
	allSigned, signResult := bot.checkCLASignResult(org, repo, number, commits, repoCnf)
	assert.Equal(t, false, allSigned)
	assert.Equal(t, ([]string)(nil), signResult[0])
	assert.Equal(t, ([]string)(nil), signResult[1])
	assert.Equal(t, ([]string)(nil), signResult[2])

	commits1 := []client.PRCommit{
		{
			"u3",
			"e3",
			"u3",
			"e3",
		},
	}
	cli.CLAState = client.CLASignStateUnknown
	// CLA sever is unavailable
	allSigned1, signResult1 := bot.checkCLASignResult(org, repo, number, commits1, repoCnf)
	assert.Equal(t, false, allSigned1)
	assert.Equal(t, ([]string)(nil), signResult1[0])
	assert.Equal(t, ([]string)(nil), signResult1[1])
	assert.Equal(t, []string{"u3"}, signResult1[2])

	cli.CLAState = client.CLASignStateNo
	// CLA sever is available, but not signed
	allSigned2, signResult2 := bot.checkCLASignResult(org, repo, number, commits1, repoCnf)
	assert.Equal(t, false, allSigned2)
	assert.Equal(t, ([]string)(nil), signResult2[0])
	assert.Equal(t, []string{"u3"}, signResult2[1])
	assert.Equal(t, ([]string)(nil), signResult2[2])

	cli.CLAState = client.CLASignStateYes
	// CLA sever is available, and signed
	allSigned3, signResult3 := bot.checkCLASignResult(org, repo, number, commits1, repoCnf)
	assert.Equal(t, true, allSigned3)
	assert.Equal(t, []string{"u3"}, signResult3[0])
	assert.Equal(t, ([]string)(nil), signResult3[1])
	assert.Equal(t, ([]string)(nil), signResult3[2])

	commits2 := []client.PRCommit{
		{
			"u0",
			"e0",
			"u0",
			"e0",
		},
	}
	cli.CLAState = client.CLASignStateYes
	// CLA sever is available, and signed, but email is invalid
	allSigned4, signResult4 := bot.checkCLASignResult(org, repo, number, commits2, repoCnf)
	assert.Equal(t, false, allSigned4)
	assert.Equal(t, ([]string)(nil), signResult4[0])
	assert.Equal(t, ([]string)(nil), signResult4[1])
	assert.Equal(t, []string{"u0"}, signResult4[2])
}
