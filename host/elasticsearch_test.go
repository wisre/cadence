// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.
//+build esintegration

// to run locally, make sure kafka and es is running,
// then run cmd `go test -v ./host -run TestElasticsearchIntegrationSuite -tags esintegration`
package host

import (
	"flag"
	"fmt"
	"io/ioutil"
	"strconv"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	workflow "github.com/uber/cadence/.gen/go/shared"
	"github.com/uber/cadence/common"
)

const (
	numOfRetry   = 50
	waitTimeInMs = 400
)

type elasticsearchIntegrationSuite struct {
	// override suite.Suite.Assertions with require.Assertions; this means that s.NotNil(nil) will stop the test,
	// not merely log an error
	*require.Assertions
	IntegrationBase
	esClient *elastic.Client
}

// This cluster use customized threshold for history config
func (s *elasticsearchIntegrationSuite) SetupSuite() {
	s.setupSuite("testdata/integration_elasticsearch_cluster.yaml")
	s.createESClient()
	s.putIndexTemplate("testdata/es_index_template.json", "test-visibility-template")
	s.createIndex(s.testClusterConfig.ESConfig.Indices[common.VisibilityAppName])
}

func (s *elasticsearchIntegrationSuite) TearDownSuite() {
	s.tearDownSuite()
	s.deleteIndex(s.testClusterConfig.ESConfig.Indices[common.VisibilityAppName])
}

func (s *elasticsearchIntegrationSuite) SetupTest() {
	// Have to define our overridden assertions in the test setup. If we did it earlier, s.T() will return nil
	s.Assertions = require.New(s.T())
}

func TestElasticsearchIntegrationSuite(t *testing.T) {
	flag.Parse()
	suite.Run(t, new(elasticsearchIntegrationSuite))
}

func (s *elasticsearchIntegrationSuite) TestListOpenWorkflow() {
	id := "es-integration-start-workflow-test"
	wt := "es-integration-start-workflow-test-type"
	tl := "es-integration-start-workflow-test-tasklist"
	identity := "worker1"

	workflowType := &workflow.WorkflowType{}
	workflowType.Name = common.StringPtr(wt)

	taskList := &workflow.TaskList{}
	taskList.Name = common.StringPtr(tl)

	request := &workflow.StartWorkflowExecutionRequest{
		RequestId:                           common.StringPtr(uuid.New()),
		Domain:                              common.StringPtr(s.domainName),
		WorkflowId:                          common.StringPtr(id),
		WorkflowType:                        workflowType,
		TaskList:                            taskList,
		Input:                               nil,
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(1),
		Identity:                            common.StringPtr(identity),
	}

	startTime := time.Now().UnixNano()
	we, err := s.engine.StartWorkflowExecution(createContext(), request)
	s.Nil(err)

	startFilter := &workflow.StartTimeFilter{}
	startFilter.EarliestTime = common.Int64Ptr(startTime)
	var openExecution *workflow.WorkflowExecutionInfo
	for i := 0; i < numOfRetry; i++ {
		startFilter.LatestTime = common.Int64Ptr(time.Now().UnixNano())
		resp, err := s.engine.ListOpenWorkflowExecutions(createContext(), &workflow.ListOpenWorkflowExecutionsRequest{
			Domain:          common.StringPtr(s.domainName),
			MaximumPageSize: common.Int32Ptr(100),
			StartTimeFilter: startFilter,
			ExecutionFilter: &workflow.WorkflowExecutionFilter{
				WorkflowId: common.StringPtr(id),
			},
		})
		s.Nil(err)
		if len(resp.GetExecutions()) == 1 {
			openExecution = resp.GetExecutions()[0]
			break
		}
		time.Sleep(waitTimeInMs * time.Millisecond)
	}
	s.NotNil(openExecution)
	s.Equal(we.GetRunId(), openExecution.GetExecution().GetRunId())
}

func (s *elasticsearchIntegrationSuite) TestListWorkflow() {
	id := "es-integration-list-workflow-test"
	wt := "es-integration-list-workflow-test-type"
	tl := "es-integration-list-workflow-test-tasklist"
	identity := "worker1"

	workflowType := &workflow.WorkflowType{}
	workflowType.Name = common.StringPtr(wt)

	taskList := &workflow.TaskList{}
	taskList.Name = common.StringPtr(tl)

	request := &workflow.StartWorkflowExecutionRequest{
		RequestId:                           common.StringPtr(uuid.New()),
		Domain:                              common.StringPtr(s.domainName),
		WorkflowId:                          common.StringPtr(id),
		WorkflowType:                        workflowType,
		TaskList:                            taskList,
		Input:                               nil,
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(1),
		Identity:                            common.StringPtr(identity),
	}

	s.testHelperForReadOnce(request, false)
}

func (s *elasticsearchIntegrationSuite) TestListWorkflow_PageToken() {
	id := "es-integration-list-workflow-token-test"
	wt := "es-integration-list-workflow-token-test-type"
	tl := "es-integration-list-workflow-token-test-tasklist"
	identity := "worker1"

	workflowType := &workflow.WorkflowType{}
	workflowType.Name = common.StringPtr(wt)

	taskList := &workflow.TaskList{}
	taskList.Name = common.StringPtr(tl)

	request := &workflow.StartWorkflowExecutionRequest{
		Domain:                              common.StringPtr(s.domainName),
		WorkflowType:                        workflowType,
		TaskList:                            taskList,
		Input:                               nil,
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(1),
		Identity:                            common.StringPtr(identity),
	}

	numOfWorkflows := defaultTestValueOfESIndexMaxResultWindow - 1 // == 4
	pageSize := 3

	s.testListWorkflowHelper(numOfWorkflows, pageSize, request, id, wt, false)
}

func (s *elasticsearchIntegrationSuite) TestListWorkflow_SearchAfter() {
	id := "es-integration-list-workflow-searchAfter-test"
	wt := "es-integration-list-workflow-searchAfter-test-type"
	tl := "es-integration-list-workflow-searchAfter-test-tasklist"
	identity := "worker1"

	workflowType := &workflow.WorkflowType{}
	workflowType.Name = common.StringPtr(wt)

	taskList := &workflow.TaskList{}
	taskList.Name = common.StringPtr(tl)

	request := &workflow.StartWorkflowExecutionRequest{
		Domain:                              common.StringPtr(s.domainName),
		WorkflowType:                        workflowType,
		TaskList:                            taskList,
		Input:                               nil,
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(1),
		Identity:                            common.StringPtr(identity),
	}

	numOfWorkflows := defaultTestValueOfESIndexMaxResultWindow + 1 // == 6
	pageSize := 4

	s.testListWorkflowHelper(numOfWorkflows, pageSize, request, id, wt, false)
}

func (s *elasticsearchIntegrationSuite) testListWorkflowHelper(numOfWorkflows, pageSize int,
	startRequest *workflow.StartWorkflowExecutionRequest, wid, wType string, isScan bool) {

	// start enough number of workflows
	for i := 0; i < numOfWorkflows; i++ {
		startRequest.RequestId = common.StringPtr(uuid.New())
		startRequest.WorkflowId = common.StringPtr(wid + strconv.Itoa(i))
		_, err := s.engine.StartWorkflowExecution(createContext(), startRequest)
		s.Nil(err)
	}

	var openExecutions []*workflow.WorkflowExecutionInfo
	var nextPageToken []byte

	listRequest := &workflow.ListWorkflowExecutionsRequest{
		Domain:        common.StringPtr(s.domainName),
		PageSize:      common.Int32Ptr(int32(pageSize)),
		NextPageToken: nextPageToken,
		Query:         common.StringPtr(fmt.Sprintf(`WorkflowType = '%s' and CloseTime = missing`, wType)),
	}
	// test first page
	for i := 0; i < numOfRetry; i++ {
		var resp *workflow.ListWorkflowExecutionsResponse
		var err error

		if isScan {
			resp, err = s.engine.ScanWorkflowExecutions(createContext(), listRequest)
		} else {
			resp, err = s.engine.ListWorkflowExecutions(createContext(), listRequest)
		}
		s.Nil(err)
		if len(resp.GetExecutions()) == pageSize {
			openExecutions = resp.GetExecutions()
			nextPageToken = resp.NextPageToken
			break
		}
		time.Sleep(waitTimeInMs * time.Millisecond)
	}
	s.NotNil(openExecutions)
	s.NotNil(nextPageToken)
	s.True(len(nextPageToken) > 0)

	// test last page
	listRequest.NextPageToken = nextPageToken
	inIf := false
	for i := 0; i < numOfRetry; i++ {
		var resp *workflow.ListWorkflowExecutionsResponse
		var err error

		if isScan {
			resp, err = s.engine.ScanWorkflowExecutions(createContext(), listRequest)
		} else {
			resp, err = s.engine.ListWorkflowExecutions(createContext(), listRequest)
		}
		s.Nil(err)
		if len(resp.GetExecutions()) == numOfWorkflows-pageSize {
			inIf = true
			openExecutions = resp.GetExecutions()
			nextPageToken = resp.NextPageToken
			break
		}
		time.Sleep(waitTimeInMs * time.Millisecond)
	}
	s.True(inIf)
	s.NotNil(openExecutions)
	s.Nil(nextPageToken)
}

func (s *elasticsearchIntegrationSuite) testHelperForReadOnce(request *workflow.StartWorkflowExecutionRequest, isScan bool) {
	we, err := s.engine.StartWorkflowExecution(createContext(), request)
	s.Nil(err)

	var openExecution *workflow.WorkflowExecutionInfo
	listRequest := &workflow.ListWorkflowExecutionsRequest{
		Domain:   common.StringPtr(s.domainName),
		PageSize: common.Int32Ptr(100),
		Query:    common.StringPtr(fmt.Sprintf(`WorkflowID = "%s"`, request.GetWorkflowId())),
	}
	for i := 0; i < numOfRetry; i++ {
		var resp *workflow.ListWorkflowExecutionsResponse
		var err error

		if isScan {
			resp, err = s.engine.ScanWorkflowExecutions(createContext(), listRequest)
		} else {
			resp, err = s.engine.ListWorkflowExecutions(createContext(), listRequest)
		}

		s.Nil(err)
		if len(resp.GetExecutions()) == 1 {
			openExecution = resp.GetExecutions()[0]
			break
		}
		time.Sleep(waitTimeInMs * time.Millisecond)
	}
	s.NotNil(openExecution)
	s.Equal(we.GetRunId(), openExecution.GetExecution().GetRunId())
}

func (s *elasticsearchIntegrationSuite) TestScanWorkflow() {
	id := "es-integration-scan-workflow-test"
	wt := "es-integration-scan-workflow-test-type"
	tl := "es-integration-scan-workflow-test-tasklist"
	identity := "worker1"

	workflowType := &workflow.WorkflowType{}
	workflowType.Name = common.StringPtr(wt)

	taskList := &workflow.TaskList{}
	taskList.Name = common.StringPtr(tl)

	request := &workflow.StartWorkflowExecutionRequest{
		RequestId:                           common.StringPtr(uuid.New()),
		Domain:                              common.StringPtr(s.domainName),
		WorkflowId:                          common.StringPtr(id),
		WorkflowType:                        workflowType,
		TaskList:                            taskList,
		Input:                               nil,
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(1),
		Identity:                            common.StringPtr(identity),
	}

	s.testHelperForReadOnce(request, true)
}

func (s *elasticsearchIntegrationSuite) TestScanWorkflow_PageToken() {
	id := "es-integration-scan-workflow-token-test"
	wt := "es-integration-scan-workflow-token-test-type"
	tl := "es-integration-scan-workflow-token-test-tasklist"
	identity := "worker1"

	workflowType := &workflow.WorkflowType{}
	workflowType.Name = common.StringPtr(wt)

	taskList := &workflow.TaskList{}
	taskList.Name = common.StringPtr(tl)

	request := &workflow.StartWorkflowExecutionRequest{
		Domain:                              common.StringPtr(s.domainName),
		WorkflowType:                        workflowType,
		TaskList:                            taskList,
		Input:                               nil,
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(1),
		Identity:                            common.StringPtr(identity),
	}

	numOfWorkflows := 4
	pageSize := 3

	s.testListWorkflowHelper(numOfWorkflows, pageSize, request, id, wt, true)
}

func (s *elasticsearchIntegrationSuite) TestCountWorkflow() {
	id := "es-integration-count-workflow-test"
	wt := "es-integration-count-workflow-test-type"
	tl := "es-integration-count-workflow-test-tasklist"
	identity := "worker1"

	workflowType := &workflow.WorkflowType{}
	workflowType.Name = common.StringPtr(wt)

	taskList := &workflow.TaskList{}
	taskList.Name = common.StringPtr(tl)

	request := &workflow.StartWorkflowExecutionRequest{
		RequestId:                           common.StringPtr(uuid.New()),
		Domain:                              common.StringPtr(s.domainName),
		WorkflowId:                          common.StringPtr(id),
		WorkflowType:                        workflowType,
		TaskList:                            taskList,
		Input:                               nil,
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(100),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(1),
		Identity:                            common.StringPtr(identity),
	}

	_, err := s.engine.StartWorkflowExecution(createContext(), request)
	s.Nil(err)

	countRequest := &workflow.CountWorkflowExecutionsRequest{
		Domain: common.StringPtr(s.domainName),
		Query:  common.StringPtr(fmt.Sprintf(`WorkflowID = "%s"`, request.GetWorkflowId())),
	}
	var resp *workflow.CountWorkflowExecutionsResponse
	for i := 0; i < numOfRetry; i++ {
		resp, err = s.engine.CountWorkflowExecutions(createContext(), countRequest)
		s.Nil(err)
		if resp.GetCount() == int64(1) {
			break
		}
		time.Sleep(waitTimeInMs * time.Millisecond)
	}
	s.Equal(int64(1), resp.GetCount())
}

func (s *elasticsearchIntegrationSuite) createESClient() {
	var err error
	s.esClient, err = elastic.NewClient(
		elastic.SetURL(s.testClusterConfig.ESConfig.URL.String()),
		elastic.SetRetrier(elastic.NewBackoffRetrier(elastic.NewExponentialBackoff(128*time.Millisecond, 513*time.Millisecond))),
	)
	s.Require().NoError(err)
}

func (s *elasticsearchIntegrationSuite) putIndexTemplate(templateConfigFile, templateName string) {
	template, err := ioutil.ReadFile(templateConfigFile)
	s.Require().NoError(err)
	putTemplate, err := s.esClient.IndexPutTemplate(templateName).BodyString(string(template)).Do(createContext())
	s.Require().NoError(err)
	s.Require().True(putTemplate.Acknowledged)
}

func (s *elasticsearchIntegrationSuite) createIndex(indexName string) {
	exists, err := s.esClient.IndexExists(indexName).Do(createContext())
	s.Require().NoError(err)
	if exists {
		deleteTestIndex, err := s.esClient.DeleteIndex(indexName).Do(createContext())
		s.Require().Nil(err)
		s.Require().True(deleteTestIndex.Acknowledged)
	}

	createTestIndex, err := s.esClient.CreateIndex(indexName).Do(createContext())
	s.Require().NoError(err)
	s.Require().True(createTestIndex.Acknowledged)
}

func (s *elasticsearchIntegrationSuite) deleteIndex(indexName string) {
	deleteTestIndex, err := s.esClient.DeleteIndex(indexName).Do(createContext())
	s.Nil(err)
	s.True(deleteTestIndex.Acknowledged)
}
