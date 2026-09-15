package ask

import (
	"fmt"
	"strings"
)

const displayReservedRows = 3

func EstimateDisplayCapacity(columns, rows int) int {
	if columns < 20 || rows <= displayReservedRows {
		return 0
	}
	cells := (rows - displayReservedRows) * columns
	return cells - cells/5
}

func withDisplayBudget(question string, capacity int) string {
	if capacity <= 0 {
		return question
	}
	budget := fmt.Sprintf(
		"[Terminal display budget: this run can display about %d character cells. "+
			"Keep the final answer within this budget unless the user explicitly asks for a long or comprehensive response.]",
		capacity,
	)
	return strings.TrimRight(question, " \t\r\n") + "\n\n" + budget
}
