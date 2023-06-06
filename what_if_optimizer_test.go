package main

import (
	"fmt"
	"testing"
)

func TestWhatIfOptimizer(t *testing.T) {
	dsn := "root:@tcp(127.0.0.1:4000)/test"
	o, err := NewTiDBWhatIfOptimizer(dsn)
	must(err)
	defer o.Close()
	must(o.Execute(`create table t (a int, b int)`))
	p1, err := o.Explain(`select * from t where a=1`)
	must(err)
	must(o.CreateHypoIndex(NewIndex("test", "t", "idx_a", "a")))
	p2, err := o.Explain(`select * from t where a=1`)
	must(err)
	must(o.DropHypoIndex(NewIndex("test", "t", "idx_a", "a")))
	p3, err := o.Explain(`select * from t where a=1`)
	must(err)
	fmt.Println(p1.PlanCost(), p2.PlanCost(), p3.PlanCost()) // cost2 > cost1 = cost3
}
