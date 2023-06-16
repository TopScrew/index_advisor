package workload

import (
	"fmt"
	"github.com/qw4990/index_advisor/utils"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/pingcap/parser/types"
)

type SQLType int

const (
	SQLTypeSelect SQLType = iota
	SQLTypeInsert
	SQLTypeUpdate
	SQLTypeOthers
)

type SQL struct { // DQL or DML
	Alias            string
	SchemaName       string
	Text             string
	Frequency        int
	IndexableColumns utils.Set[Column] // Indexable columns related to this SQL
	Plans            []Plan            // A SQL may have multiple different plans
}

func (sql SQL) Type() SQLType {
	text := strings.TrimSpace(sql.Text)
	if len(text) < 6 {
		return SQLTypeOthers
	}
	prefix := strings.ToLower(text[:6])
	if strings.HasPrefix(prefix, "select") {
		return SQLTypeSelect
	}
	if strings.HasPrefix(prefix, "insert") {
		return SQLTypeInsert
	}
	if strings.HasPrefix(prefix, "update") {
		return SQLTypeUpdate
	}
	return SQLTypeOthers
}

func (sql SQL) Key() string {
	return sql.Text
}

type TableSchema struct {
	SchemaName     string
	TableName      string
	Columns        []Column
	Indexes        []Index
	CreateStmtText string // `create table t (...)`
}

func (t TableSchema) Key() string {
	return fmt.Sprintf("%v.%v", t.SchemaName, t.TableName)
}

type TableStats struct {
	SchemaName    string
	TableName     string
	StatsFilePath string
}

func (t TableStats) Key() string {
	return fmt.Sprintf("%v.%v", t.SchemaName, t.TableName)
}

type Column struct {
	SchemaName string
	TableName  string
	ColumnName string
	ColumnType *types.FieldType
}

func NewColumn(schemaName, tableName, columnName string) Column {
	return Column{SchemaName: strings.ToLower(schemaName), TableName: strings.ToLower(tableName), ColumnName: strings.ToLower(columnName)}
}

func NewColumns(schemaName, tableName string, columnNames ...string) []Column {
	var cols []Column
	for _, col := range columnNames {
		cols = append(cols, NewColumn(schemaName, tableName, col))
	}
	return cols
}

func (c Column) Key() string {
	return fmt.Sprintf("%v.%v.%v", c.SchemaName, c.TableName, c.ColumnName)
}

func (c Column) String() string {
	return fmt.Sprintf("%v.%v.%v", c.SchemaName, c.TableName, c.ColumnName)
}

type Index struct {
	SchemaName string
	TableName  string
	IndexName  string
	Columns    []Column
}

func NewIndex(schemaName, tableName, indexName string, columns ...string) Index {
	return Index{SchemaName: strings.ToLower(schemaName), TableName: strings.ToLower(tableName), IndexName: strings.ToLower(indexName), Columns: NewColumns(schemaName, tableName, columns...)}
}

func (i Index) ColumnNames() []string {
	var names []string
	for _, col := range i.Columns {
		names = append(names, col.ColumnName)
	}
	return names
}

func (i Index) DDL() string {
	return fmt.Sprintf("CREATE INDEX %v ON %v.%v (%v)", i.IndexName, i.SchemaName, i.TableName, strings.Join(i.ColumnNames(), ", "))
}

func (i Index) Key() string {
	return fmt.Sprintf("%v.%v(%v)", i.SchemaName, i.TableName, strings.Join(i.ColumnNames(), ","))
}

// PrefixContain returns whether j is a prefix of i.
func (i Index) PrefixContain(j Index) bool {
	if i.SchemaName != j.SchemaName || i.TableName != j.TableName || len(i.Columns) < len(j.Columns) {
		return false
	}
	for k := range j.Columns {
		if i.Columns[k].ColumnName != j.Columns[k].ColumnName {
			return false
		}
	}
	return true
}

type Plan struct {
	Plan [][]string
}

// IsExecuted returns whether this plan is executed.
func (p Plan) IsExecuted() bool {
	// | id | estRows  | estCost | actRows | task | access object | execution info | operator info | memory | disk |
	return len(p.Plan[0]) == 10
}

func (p Plan) PlanCost() float64 {
	v, err := strconv.ParseFloat(p.Plan[0][2], 64)
	utils.Must(err)
	return v
}

func (p Plan) ExecTime() time.Duration {
	if !p.IsExecuted() {
		return 0
	}

	//| TableReader_5 | 10000.00 | 177906.67 | 0 | root | - | time:3.15ms, loops:1, ... | data:TableFullScan_4 | 174 Bytes | N/A |
	execInfo := p.Plan[0][6]
	b := strings.Index(execInfo, "time:")
	e := strings.Index(execInfo, ",")
	tStr := execInfo[b+len("time:") : e]
	d, err := time.ParseDuration(tStr)
	utils.Must(err)
	return d
}

type SampleRows struct {
	TableName string
}

type WorkloadInfo struct {
	SQLs             utils.Set[SQL]
	TableSchemas     utils.Set[TableSchema]
	TableStats       utils.Set[TableStats]
	IndexableColumns utils.Set[Column]
	SampleRows       []SampleRows
}

// AllSchemaNames returns all schema names in this workload.
func (w WorkloadInfo) AllSchemaNames() []string {
	x := make(map[string]struct{})
	result := make([]string, 0)
	for _, t := range w.TableSchemas.ToList() {
		if _, ok := x[t.SchemaName]; !ok {
			result = append(result, t.SchemaName)
			x[t.SchemaName] = struct{}{}
		}
	}
	return result
}

// IndexConfCost is the cost of a index configuration.
type IndexConfCost struct {
	TotalWorkloadQueryCost    float64
	TotalNumberOfIndexColumns int
}

func (c IndexConfCost) Less(other IndexConfCost) bool {
	if c.TotalNumberOfIndexColumns == 0 { // not initialized
		return false
	}
	if other.TotalNumberOfIndexColumns == 0 { // not initialized
		return true
	}
	cc, cOther := c.TotalWorkloadQueryCost, other.TotalWorkloadQueryCost
	if math.Abs(cc-cOther) < 10 || math.Abs(cc-cOther)/math.Max(cc, cOther) < 0.01 {
		// if they have the same cost, then the less columns, the better.
		return c.TotalNumberOfIndexColumns < other.TotalNumberOfIndexColumns
	}
	return c.TotalWorkloadQueryCost < other.TotalWorkloadQueryCost
}