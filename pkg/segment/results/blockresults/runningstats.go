/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package blockresults

import (
	"fmt"

	"github.com/axiomhq/hyperloglog"
	"github.com/siglens/siglens/pkg/segment/structs"
	"github.com/siglens/siglens/pkg/segment/utils"
	log "github.com/sirupsen/logrus"
)

type RunningBucketResults struct {
	runningStats        []runningStats               // maps a stat name to running stats
	currStats           []*structs.MeasureAggregator // measure aggregators in result
	groupedRunningStats map[string][]runningStats    // maps timechart group by col's vals to corresponding running stats
	count               uint64                       // total number of elements belonging to the bucket
}

type runningStats struct {
	rawVal utils.CValueEnclosure // raw value
	hll    *hyperloglog.Sketch
}

func initRunningStats(internalMeasureFns []*structs.MeasureAggregator) []runningStats {
	retVal := make([]runningStats, len(internalMeasureFns))
	for i := 0; i < len(internalMeasureFns); i++ {
		if internalMeasureFns[i].MeasureFunc == utils.Cardinality {
			retVal[i] = runningStats{hll: hyperloglog.New()}
		}
	}
	return retVal
}

func initRunningGroupByBucket(internalMeasureFns []*structs.MeasureAggregator) *RunningBucketResults {

	return &RunningBucketResults{
		count:               0,
		runningStats:        initRunningStats(internalMeasureFns),
		currStats:           internalMeasureFns,
		groupedRunningStats: make(map[string][]runningStats),
	}
}

func initRunningTimeBucket() *RunningBucketResults {

	return &RunningBucketResults{
		count: 0,
	}
}

func (rr *RunningBucketResults) AddTimeToBucketStats(count uint16) {
	rr.count += uint64(count)
}

func (rr *RunningBucketResults) AddMeasureResults(runningStats *[]runningStats, measureResults []utils.CValueEnclosure, qid uint64,
	cnt uint64, usedByTimechart bool) {
	for i := 0; i < len(*runningStats); i++ {
		switch rr.currStats[i].MeasureFunc {
		case utils.Sum:
			fallthrough
		case utils.Max:
			fallthrough
		case utils.Min:
			err := rr.AddEvalResultsForMinOrMaxOrSum(runningStats, measureResults, i)
			if err != nil {
				log.Errorf("AddMeasureResults: %v", err)
			}
		case utils.Count:
			step, err := rr.AddEvalResultsForCount(runningStats, measureResults, i, usedByTimechart, cnt)
			if err != nil {
				log.Errorf("AddMeasureResults: %v", err)
			}
			i += step
		case utils.Cardinality:
			if rr.currStats[i].ValueColRequest == nil {
				rawVal, err := measureResults[i].GetUIntValue()
				if err != nil {
					log.Errorf("AddMeasureResults: failed to add measurement to running stats: %v", err)
					continue
				}
				(*runningStats)[i].hll.InsertHash(rawVal)
				continue
			}
			fallthrough
		case utils.Values:
			step, err := rr.AddEvalResultsForValuesOrCardinality(runningStats, measureResults, i)
			if err != nil {
				log.Errorf("AddMeasureResults: %v", err)
			}
			i += step
		default:
			err := rr.ProcessReduce(runningStats, measureResults[i], i)
			if err != nil {
				log.Errorf("AddMeasureResults: %v", err)
			}
		}
	}
	rr.count += cnt
}

// This assumes the order of bucketResults.RunningStats are in the same order, referencing the same measure request
func (rr *RunningBucketResults) MergeRunningBuckets(toJoin *RunningBucketResults) {

	if toJoin == nil {
		return
	}

	// Merge group by bucket inside each time range bucket (For timechart)
	if rr.groupedRunningStats != nil && len(rr.groupedRunningStats) > 0 {
		for groupByColVal, runningStats := range rr.groupedRunningStats {
			toJoinRunningStats, exists := toJoin.groupedRunningStats[groupByColVal]
			if !exists {
				log.Errorf("MergeRunningBuckets: groupByColVal: %v does not have a runningStats", groupByColVal)
				continue
			}
			rr.mergeRunningStats(&runningStats, toJoinRunningStats)
		}
	}

	rr.mergeRunningStats(&rr.runningStats, toJoin.runningStats)
	rr.count += toJoin.count
}

func (rr *RunningBucketResults) mergeRunningStats(runningStats *[]runningStats, toJoinRunningStats []runningStats) {
	for i := 0; i < len(toJoinRunningStats); i++ {
		switch rr.currStats[i].MeasureFunc {
		case utils.Values:
			if rr.currStats[i].ValueColRequest == nil {
				err := rr.ProcessReduce(runningStats, toJoinRunningStats[i].rawVal, i)
				if err != nil {
					log.Errorf("mergeRunningStats: err: %v", err)
				}
			} else {
				fields := rr.currStats[i].ValueColRequest.GetFields()
				err := rr.ProcessReduce(runningStats, toJoinRunningStats[i].rawVal, i)
				if err != nil {
					log.Errorf("mergeRunningStats: err: %v", err)
				}
				i += (len(fields) - 1)
			}
		case utils.Cardinality:
			if rr.currStats[i].ValueColRequest == nil {
				err := (*runningStats)[i].hll.Merge(toJoinRunningStats[i].hll)
				if err != nil {
					log.Errorf("mergeRunningStats: failed merge HLL!: %v", err)
				}
			} else {
				fields := rr.currStats[i].ValueColRequest.GetFields()
				err := rr.ProcessReduce(runningStats, toJoinRunningStats[i].rawVal, i)
				if err != nil {
					log.Errorf("mergeRunningStats: err: %v", err)
				}
				i += (len(fields) - 1)
			}
		case utils.Count:
			if rr.currStats[i].ValueColRequest == nil {
				err := rr.ProcessReduce(runningStats, toJoinRunningStats[i].rawVal, i)
				if err != nil {
					log.Errorf("mergeRunningStats: err: %v", err)
				}
			} else {
				fields := rr.currStats[i].ValueColRequest.GetFields()
				err := rr.ProcessReduce(runningStats, toJoinRunningStats[i].rawVal, i)
				if err != nil {
					log.Errorf("mergeRunningStats: failed to add measurement to running stats: %v", err)
				}
				i += (len(fields) - 1)
			}
		default:
			err := rr.ProcessReduce(runningStats, toJoinRunningStats[i].rawVal, i)
			if err != nil {
				log.Errorf("mergeRunningStats: err: %v", err)
			}
		}
	}
}

func (rr *RunningBucketResults) ProcessReduce(runningStats *[]runningStats, e utils.CValueEnclosure, i int) error {
	retVal, err := utils.Reduce((*runningStats)[i].rawVal, e, rr.currStats[i].MeasureFunc)
	if err != nil {
		return fmt.Errorf("ProcessReduce: failed to add measurement to running stats: %v", err)
	} else {
		(*runningStats)[i].rawVal = retVal
	}
	return nil
}

func (rr *RunningBucketResults) AddEvalResultsForMinOrMaxOrSum(runningStats *[]runningStats, measureResults []utils.CValueEnclosure, i int) error {
	if rr.currStats[i].ValueColRequest == nil {
		return rr.ProcessReduce(runningStats, measureResults[i], i)
	}

	fields := rr.currStats[i].ValueColRequest.GetFields()
	if len(fields) != 1 {
		return fmt.Errorf("AddEvalResultsForMinOrMaxOrSum: Incorrect number of fields for aggCol: %v", rr.currStats[i].String())
	}
	fieldToValue := make(map[string]utils.CValueEnclosure)
	fieldToValue[fields[0]] = measureResults[i]
	boolResult, err := rr.currStats[i].ValueColRequest.BooleanExpr.Evaluate(fieldToValue)
	if err != nil {
		return fmt.Errorf("AddEvalResultsForMinOrMaxOrSum: there are some errors in the eval function that is inside the min/max function: %v", err)
	}
	if boolResult {
		err := rr.ProcessReduce(runningStats, measureResults[i], i)
		if err != nil {
			return fmt.Errorf("AddEvalResultsForMinOrMaxOrSum: %v", err)
		}
	}
	return nil
}

func (rr *RunningBucketResults) AddEvalResultsForCount(runningStats *[]runningStats, measureResults []utils.CValueEnclosure, i int, usedByTimechart bool, cnt uint64) (int, error) {

	if rr.currStats[i].ValueColRequest == nil {
		if usedByTimechart {
			eVal := &utils.CValueEnclosure{
				Dtype: utils.SS_DT_UNSIGNED_NUM,
				CVal:  cnt,
			}
			return 0, rr.ProcessReduce(runningStats, *eVal, i)
		} else {
			return 0, rr.ProcessReduce(runningStats, measureResults[i], i)
		}
	}

	fields := rr.currStats[i].ValueColRequest.GetFields()
	fieldToValue := make(map[string]utils.CValueEnclosure)

	index := i
	for _, field := range fields {
		fieldToValue[field] = measureResults[index]
		index++
	}

	boolResult, err := rr.currStats[i].ValueColRequest.BooleanExpr.Evaluate(fieldToValue)
	if err != nil {
		return 0, fmt.Errorf("AddEvalResultsForCount: there are some errors in the eval function that is inside the count function: %v", err)
	}
	if (*runningStats)[i].rawVal.CVal == nil {
		(*runningStats)[i].rawVal = utils.CValueEnclosure{
			CVal:  int64(0),
			Dtype: utils.SS_DT_SIGNED_NUM,
		}
	}
	if boolResult {
		(*runningStats)[i].rawVal.CVal = (*runningStats)[i].rawVal.CVal.(int64) + 1
	}

	return len(fields) - 1, nil
}

func (rr *RunningBucketResults) AddEvalResultsForValuesOrCardinality(runningStats *[]runningStats, measureResults []utils.CValueEnclosure, i int) (int, error) {
	if (*runningStats)[i].rawVal.CVal == nil {
		(*runningStats)[i].rawVal = utils.CValueEnclosure{
			Dtype: utils.SS_DT_STRING_SET,
			CVal:  make(map[string]struct{}, 0),
		}
	}
	strSet := (*runningStats)[i].rawVal.CVal.(map[string]struct{})

	if rr.currStats[i].ValueColRequest == nil {
		strVal, err := measureResults[i].GetString()
		if err != nil {
			return 0, fmt.Errorf("AddEvalResultsForValuesOrCardinality: failed to add measurement to running stats: %v", err)
		}
		strSet[strVal] = struct{}{}
		(*runningStats)[i].rawVal.CVal = strSet
		return 0, nil
	}

	fields := rr.currStats[i].ValueColRequest.GetFields()
	fieldToValue := make(map[string]utils.CValueEnclosure)

	index := i
	for _, field := range fields {
		fieldToValue[field] = measureResults[index]
		index++
	}

	strVal, err := rr.currStats[i].ValueColRequest.EvaluateToString(fieldToValue)
	if err != nil {
		return 0, fmt.Errorf("AddEvalResultsForValuesOrCardinality: there are some errors in the eval function that is inside the count function: %v", err)
	}
	strSet[strVal] = struct{}{}
	(*runningStats)[i].rawVal.CVal = strSet

	return len(fields) - 1, nil
}
