package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/npdms/api/internal/models"
)

// DatabaseRefusal turns a rule the database enforced into the status and the
// sentence that rule deserves.
//
// Every one of these used to surface as a generic 500. A value outside an
// enum, a duplicate record number, a reference to something that is not there,
// a trigger saying a court order is never deleted — all of them are the caller
// being told no, and all of them arrived as "server error", which reads as a
// fault in the platform and sends an officer to the wrong person for help.
// POA.md records the cost of this plainly: every defect found in the registers
// surfaced only as a generic 500, and logging the cause would have shown each
// in seconds.
//
// Returns false when the error is not the database refusing, so the caller can
// fall back to its own handling.
func DatabaseRefusal(c *gin.Context, err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	switch pgErr.Code {
	case "23514": // check constraint
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "value_not_allowed",
			Message: checkConstraintMessage(pgErr),
			Code:    400,
		})
	case "23505": // unique violation
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "already_exists",
			Message: "That record already exists. " + detailOf(pgErr),
			Code:    409,
		})
	case "23503": // foreign key
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "no_such_record",
			Message: "This refers to a record that does not exist. " + detailOf(pgErr),
			Code:    400,
		})
	case "23502": // not null
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "missing_value",
			Message: "\"" + pgErr.ColumnName + "\" is required and was not given.",
			Code:    400,
		})
	case "22P02", "22007", "22008": // bad input syntax, bad date
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "value_not_understood",
			Message: "A value could not be read: " + pgErr.Message,
			Code:    400,
		})
	case "P0001", "2F003", "23001": // raised by a trigger, restrict violation
		// A rule the platform states in words. The trigger's own message is
		// written for an officer to read — "a court order is a record of what
		// a court directed and is not deleted" — so it is passed through.
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "rule_refused",
			Message: pgErr.Message,
			Code:    409,
		})
	default:
		return false
	}
	return true
}

// checkConstraintMessage names the rule and, where the constraint is an enum
// check, the values it does accept.
func checkConstraintMessage(pgErr *pgconn.PgError) string {
	if pgErr.ConstraintName == "" {
		return "That value is not allowed here."
	}
	// court_orders_order_type_check -> "order type"
	field := strings.TrimSuffix(pgErr.ConstraintName, "_check")
	for _, table := range []string{"court_orders_", "firs_", "bail_", "evidence_", "forensics_", "cases_", "users_"} {
		field = strings.TrimPrefix(field, table)
	}
	field = strings.ReplaceAll(field, "_", " ")
	return "That value is not allowed for " + field + "."
}

func detailOf(pgErr *pgconn.PgError) string {
	if pgErr.Detail == "" {
		return ""
	}
	return pgErr.Detail
}
