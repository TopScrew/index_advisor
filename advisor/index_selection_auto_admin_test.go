package advisor

import (
	"fmt"
	"github.com/qw4990/index_advisor/optimizer"
	"github.com/qw4990/index_advisor/utils"
	wk "github.com/qw4990/index_advisor/workload"
	"sort"
	"strings"
	"testing"
)

func prepareTestWorkload(dsn, schemaName string, createTableStmts, rawSQLs []string) (wk.WorkloadInfo, optimizer.WhatIfOptimizer) {
	w := wk.CreateWorkloadFromRawStmt(schemaName, createTableStmts, rawSQLs)
	utils.Must(IndexableColumnsSelectionSimple(&w))
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:4000)/"
	}
	opt, err := optimizer.NewTiDBWhatIfOptimizer("root:@tcp(127.0.0.1:4000)/")
	utils.Must(err)
	//opt.SetDebug(true)

	for _, schemaName := range w.AllSchemaNames() {
		utils.Must(opt.Execute("drop database if exists " + schemaName))
		utils.Must(opt.Execute("create database " + schemaName))
	}
	for _, t := range w.TableSchemas.ToList() {
		utils.Must(opt.Execute("use " + t.SchemaName))
		utils.Must(opt.Execute(t.CreateStmtText))
	}
	return w, opt
}

type indexSelectionCase struct {
	numIndexes       int
	schemaName       string
	createTableStmts []string
	rawSQLs          []string
	expectedIndexes  []wk.Index
}

func testIndexSelection(dsn string, cases []indexSelectionCase) {
	for i, c := range cases {
		fmt.Printf("======================= case %v =======================\n", i)
		w, opt := prepareTestWorkload(dsn, c.schemaName, c.createTableStmts, c.rawSQLs)
		res, err := SelectIndexAAAlgo(w, Parameter{MaximumIndexesToRecommend: c.numIndexes}, opt)
		utils.Must(err)
		indexList := res.ToList()

		notEqual := false
		if len(c.expectedIndexes) != len(indexList) {
			notEqual = true
		} else {
			sort.Slice(indexList, func(i, j int) bool { return indexList[i].Key() < indexList[j].Key() })
			sort.Slice(c.expectedIndexes, func(i, j int) bool { return c.expectedIndexes[i].Key() < c.expectedIndexes[j].Key() })
			for i := range c.expectedIndexes {
				if c.expectedIndexes[i].Key() != indexList[i].Key() {
					notEqual = true
				}
			}
		}

		if notEqual {
			originalCost := EvaluateIndexConfCost(w, opt, utils.NewSet[wk.Index]())
			expectedCost := EvaluateIndexConfCost(w, opt, utils.ListToSet(c.expectedIndexes...))
			actualCost := EvaluateIndexConfCost(w, opt, utils.ListToSet(indexList...))
			fmt.Printf("original cost: %.2E, expected cost: %.2E, actual cost: %.2E\n",
				originalCost.TotalWorkloadQueryCost, expectedCost.TotalWorkloadQueryCost, actualCost.TotalWorkloadQueryCost)
			fmt.Printf("expected: %v\n", c.expectedIndexes)
			fmt.Printf("actual: %v\n", indexList)
			panic("")
		}
	}
}

func TestSimulateAndCost(t *testing.T) {
	_, opt := prepareTestWorkload("", "test",
		[]string{"create table t (a int, b int, c int, d int , e int)"},
		[]string{
			"select * from t where a = 1 and c = 1",
			"select * from t where b = 1 and e = 1",
		})

	opt.CreateHypoIndex(wk.NewIndex("test", "t", "a", "a"))
	plan1, _ := opt.Explain("select * from t where a = 1 and c < 1")
	opt.DropHypoIndex(wk.NewIndex("test", "t", "a", "a"))

	for _, p := range plan1.Plan {
		fmt.Println(">> ", p)
	}

	opt.CreateHypoIndex(wk.NewIndex("test", "t", "ac", "a", "c"))
	plan2, _ := opt.Explain("select * from t where a = 1 and c < 1")
	opt.DropHypoIndex(wk.NewIndex("test", "t", "ac", "a", "c"))
	for _, p := range plan2.Plan {
		fmt.Println(">> ", p)
	}
}

func TestIndexSelectionAACase(t *testing.T) {
	cases := []indexSelectionCase{
		{
			1, "test", []string{
				"create table t (a int, b int, c int)",
			}, []string{
				"select * from t where a = 1",
			}, []wk.Index{
				newIndex4Test("test.t(a)"),
			},
		},
		{
			2, "test", []string{
				"create table t (a int, b int, c int)",
			}, []string{
				"select * from t where a = 1",
			}, []wk.Index{
				newIndex4Test("test.t(a)"), // only 1 index even if we ask for 2
			},
		},
		{
			1, "test", []string{
				"create table t (a int, b int, c int)",
			}, []string{
				"select * from t where a = 1",
				"select * from t where a = 2",
				"select * from t where b = 1",
			}, []wk.Index{
				newIndex4Test("test.t(a)"),
			},
		},
		{
			1, "test", []string{
				"create table t (a int, b int, c int)",
			}, []string{
				"select * from t where a = 1",
				"select * from t where a = 2",
				"select * from t where b = 1 and a = 1",
			}, []wk.Index{
				newIndex4Test("test.t(a,b)"),
			},
		},
		{
			2, "test", []string{
				"create table t (a int, b int, c int)",
			}, []string{
				"select * from t where a = 1",
				"select * from t where a = 2",
				"select * from t where b = 1 and a = 1",
			}, []wk.Index{
				newIndex4Test("test.t(a,b)"), // only ab is recommended even if we ask for 2
			},
		},
		{
			1, "test", []string{
				"create table t (a int, b int, c int, key(a))",
			}, []string{
				"select * from t where a = 1",
				"select * from t where a = 2",
				"select * from t where b = 1",
			}, []wk.Index{
				newIndex4Test("test.t(b)"),
			},
		},
		{
			10, "test", []string{
				"create table t (a int, b int, c int)",
			}, []string{
				"select * from t where a = 1",
				"select * from t where a = 2",
				"select * from t where b = 1",
			}, []wk.Index{
				newIndex4Test("test.t(a)"),
				newIndex4Test("test.t(b)"),
			},
		},
		//{ // https://github.com/pingcap/tidb/issues/44376
		//	2, "test", []string{
		//		"create table t (a int, b int, c int, d int , e int)",
		//	}, []string{
		//		"select * from t where a = 1 and c < 1",
		//		"select * from t where b = 1 and e < 1",
		//	}, []Index{
		//		newIndex4Test("test.t(a,c)"),
		//		newIndex4Test("test.t(b,e)"),
		//	},
		//},
	}
	testIndexSelection("", cases)
}

func newIndex4Test(key string) wk.Index {
	// test.t(b)
	tmp := strings.Split(key, ".")
	schemaName := tmp[0]
	tmp = strings.Split(tmp[1], "(")
	tableName := tmp[0]
	cols := tmp[1][:len(tmp[1])-1]
	colNames := strings.Split(cols, ",")
	return wk.NewIndex(schemaName, tableName, fmt.Sprintf("%v_%v_%v", schemaName, tableName, strings.Join(colNames, "_")), colNames...)
}