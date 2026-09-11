package fuzz_tests

import (
	"context"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/chaos_experiment_run"
	store "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/data-store"
	"github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb"
	dbChaosExperiment "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/chaos_experiment"
	dbChaosExperimentRun "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/chaos_experiment_run"
	dbChaosInfra "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/chaos_infrastructure"
	dbMocks "github.com/litmuschaos/litmus/chaoscenter/graphql/server/pkg/database/mongodb/mocks"
)

type MockServices struct {
	ChaosExperimentOperator     *dbChaosExperiment.Operator
	ChaosExperimentRunOperator  *dbChaosExperimentRun.Operator
	ChaosInfrastructureOperator *dbChaosInfra.Operator
	MongodbOperator             *dbMocks.MongoOperator
	ChaosExperimentRunService   chaos_experiment_run.Service
}

func NewMockServices() *MockServices {
	var (
		mongodbMockOperator                                      = new(dbMocks.MongoOperator)
		chaosExperimentOperator                                  = dbChaosExperiment.NewChaosExperimentOperator(mongodbMockOperator)
		chaosExperimentRunOperator                               = dbChaosExperimentRun.NewChaosExperimentRunOperator(mongodbMockOperator)
		chaosInfrastructureOperator                              = dbChaosInfra.NewInfrastructureOperator(mongodbMockOperator)
		chaosExperimentRunService   chaos_experiment_run.Service = chaos_experiment_run.NewChaosExperimentRunService(
			chaosExperimentOperator,
			chaosInfrastructureOperator,
			chaosExperimentRunOperator,
		)
	)
	return &MockServices{
		ChaosExperimentOperator:     chaosExperimentOperator,
		ChaosExperimentRunOperator:  chaosExperimentRunOperator,
		ChaosInfrastructureOperator: chaosInfrastructureOperator,
		MongodbOperator:             mongodbMockOperator,
		ChaosExperimentRunService:   chaosExperimentRunService,
	}
}

func mod(raw, n int) int {
	m := raw % n
	if m < 0 {
		m += n
	}
	return m
}

func FuzzProcessExperimentRunStop(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			Query      bson.D
			Experiment dbChaosExperiment.ChaosExperimentRequest
			Username   string
			ProjectID  string
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		branchRaw, err := fuzzConsumer.GetInt()
		if err != nil {
			return
		}
		failAtRaw, err := fuzzConsumer.GetInt()
		if err != nil {
			return
		}

		var (
			experimentRunID string
			notifyID        *string
			numCalls        int
		)
		notify := uuid.NewString()
		switch mod(branchRaw, 3) {
		case 0:
			numCalls = 1
		case 1:
			experimentRunID = uuid.NewString()
			numCalls = 2
		case 2:
			notifyID = &notify
			numCalls = 2
		}

		failAt := mod(failAtRaw, numCalls+1)

		mockServices := NewMockServices()

		var runUpdateErr error
		if failAt == 0 {
			runUpdateErr = context.Canceled
		}
		mockServices.MongodbOperator.On("Update", mock.Anything, mongodb.ChaosExperimentRunsCollection, mock.Anything, mock.Anything, mock.Anything).Return(&mongo.UpdateResult{}, runUpdateErr).Once()

		if numCalls == 2 && failAt != 0 {
			var expUpdateErr error
			if failAt == 1 {
				expUpdateErr = context.Canceled
			}
			mockServices.MongodbOperator.On("Update", mock.Anything, mongodb.ChaosExperimentCollection, mock.Anything, mock.Anything, mock.Anything).Return(&mongo.UpdateResult{}, expUpdateErr).Once()
		}

		err = mockServices.ChaosExperimentRunService.ProcessExperimentRunStop(
			context.Background(),
			targetStruct.Query,
			experimentRunID,
			notifyID,
			targetStruct.Experiment,
			targetStruct.Username,
			targetStruct.ProjectID,
			store.NewStore(),
		)

		wantErr := failAt < numCalls
		if (err != nil) != wantErr {
			t.Errorf("ProcessExperimentRunStop() error = %v, wantErr %v (branch=%d failAt=%d numCalls=%d)", err, wantErr, mod(branchRaw, 3), failAt, numCalls)
		}
	})
}

func FuzzProcessCompletedExperimentRun(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &struct {
			ExecData        chaos_experiment_run.ExecutionData
			WfID            string
			ExperimentRunID string
		}{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}

		shouldErr, err := fuzzConsumer.GetBool()
		if err != nil {
			return
		}

		var (
			singleResult *mongo.SingleResult
			opErr        error
		)
		if shouldErr {
			opErr = context.Canceled
			singleResult = mongo.NewSingleResultFromDocument(nil, nil, nil)
		} else {
			findResult := []interface{}{bson.D{
				{Key: "experiment_id", Value: targetStruct.WfID},
			}}
			singleResult = mongo.NewSingleResultFromDocument(findResult[0], nil, nil)
		}

		mockServices := NewMockServices()
		mockServices.MongodbOperator.On("Get", mock.Anything, mongodb.ChaosExperimentCollection, mock.Anything).Return(singleResult, opErr).Once()

		_, err = mockServices.ChaosExperimentRunService.ProcessCompletedExperimentRun(
			targetStruct.ExecData,
			targetStruct.WfID,
			targetStruct.ExperimentRunID,
		)
		if (err != nil) != shouldErr {
			t.Errorf("ProcessCompletedExperimentRun() error = %v, shouldErr %v", err, shouldErr)
		}
	})
}
