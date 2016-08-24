package validate

import (
	"errors"
	"fmt"
	"github.com/graniticio/granitic/types"
	"regexp"
	"strconv"
)

const NoLimit = -1

const (
	stringOpTrimCode     = "TRIM"
	stringOpHardTrimCode = "HARDTRIM"
	stringOpLenCode      = "LEN"
	stringOpInCode       = "IN"
	stringOpExtCode      = "EXT"
	stringOpOptionalCode = "OPT"
	stringOpBreakCode    = "BREAK"
	stringOpRegCode      = "REG"
)

type StringValidationOperation uint

const (
	StringOpUnsupported = iota
	StringOpTrim
	StringOpHardTrim
	StringOpLen
	StringOpIn
	StringOpExt
	StringOpOptional
	StringOpBreak
	StringOpReg
)

type trimMode uint

const (
	noTrim   = 0
	softTrim = 1
	hardTrim = 2
)

type StringValidator struct {
	DefaultErrorcode string
	operations       []*stringOperation
	minLen           int
	maxLen           int
	trim             trimMode
	optional         bool
}

func (sv *StringValidator) Validate(vc *ValidationContext) (errorCodes []string, unexpected error) {

	return nil, nil

}

func (sv *StringValidator) Break() *StringValidator {

	o := new(stringOperation)
	o.OpType = StringOpBreak

	sv.addOperation(o)

	return sv

}

func (sv *StringValidator) Length(min, max int, code ...string) *StringValidator {

	sv.minLen = min
	sv.maxLen = max

	ec := sv.chooseErrorCode(code)

	o := new(stringOperation)
	o.OpType = StringOpLen
	o.ErrCode = ec

	sv.addOperation(o)

	return sv

}

func (sv *StringValidator) In(set []string, code ...string) *StringValidator {

	ss := types.NewStringSet(set)

	ec := sv.chooseErrorCode(code)

	o := new(stringOperation)
	o.OpType = StringOpIn
	o.ErrCode = ec
	o.InSet = ss

	sv.addOperation(o)

	return sv

}

func (sv *StringValidator) HardTrim() *StringValidator {

	sv.trim = hardTrim

	return sv
}

func (sv *StringValidator) Trim() *StringValidator {

	sv.trim = softTrim

	return sv
}

func (sv *StringValidator) Optional() *StringValidator {

	sv.optional = true

	return sv
}

func (sv *StringValidator) ExternalValidation(v ExternalStringValidator, code ...string) *StringValidator {
	ec := sv.chooseErrorCode(code)

	o := new(stringOperation)
	o.OpType = StringOpExt
	o.ErrCode = ec
	o.External = v

	sv.addOperation(o)

	return sv
}

func (sv *StringValidator) Regex(r *regexp.Regexp, code ...string) *StringValidator {
	ec := sv.chooseErrorCode(code)

	o := new(stringOperation)
	o.OpType = StringOpReg
	o.ErrCode = ec
	o.Regex = r

	sv.addOperation(o)

	return sv
}

func (sv *StringValidator) addOperation(o *stringOperation) {
	if sv.operations == nil {
		sv.operations = make([]*stringOperation, 0)
	}

	sv.operations = append(sv.operations, o)
}

func (sv *StringValidator) Operation(c string) (StringValidationOperation, error) {
	switch c {
	case stringOpTrimCode:
		return StringOpTrim, nil
	case stringOpHardTrimCode:
		return StringOpHardTrim, nil
	case stringOpLenCode:
		return StringOpLen, nil
	case stringOpInCode:
		return StringOpIn, nil
	case stringOpExtCode:
		return StringOpExt, nil
	case stringOpOptionalCode:
		return StringOpOptional, nil
	case stringOpBreakCode:
		return StringOpBreak, nil
	case stringOpRegCode:
		return StringOpReg, nil
	}

	m := fmt.Sprintf("Unsupported string validation operation %s", c)
	return StringOpUnsupported, errors.New(m)

}

func (sv *StringValidator) chooseErrorCode(v []string) string {

	if len(v) > 0 {
		return v[0]
	} else {
		return sv.DefaultErrorcode
	}

}

type stringOperation struct {
	OpType   StringValidationOperation
	ErrCode  string
	InSet    *types.StringSet
	External ExternalStringValidator
	Regex    *regexp.Regexp
}

type stringValidatorBuilder struct {
	strLenRegex      *regexp.Regexp
	defaultErrorCode string
}

func (vb *stringValidatorBuilder) parseStringRule(field string, rule []string) error {

	sv := new(StringValidator)
	sv.DefaultErrorcode = DetermineDefaultErrorCode(StringRuleCode, rule, vb.defaultErrorCode)

	for _, v := range rule {

		ops := DecomposeOperation(v)
		opCode := ops[0]

		if IsTypeIndicator(StringRuleCode, opCode) {
			continue
		}

		op, err := sv.Operation(opCode)

		if err != nil {
			return err
		}

		switch op {
		case StringOpBreak:
			sv.Break()
		case StringOpLen:
			err = vb.addStringLenOperation(field, ops, sv)
		case StringOpHardTrim:
			sv.HardTrim()
		case StringOpTrim:
			sv.Trim()
		}

		if err != nil {
			return err
		}

	}

	return nil

}

func (vb *stringValidatorBuilder) addStringLenOperation(field string, ops []string, sv *StringValidator) error {

	opParams := len(ops)

	if opParams < 2 || opParams > 3 {
		m := fmt.Sprintf("Length operation for field %s is invalid", field)
		return errors.New(m)
	}

	vals := ops[1]

	if !vb.strLenRegex.MatchString(vals) {
		m := fmt.Sprintf("Length parameters for field %s are invalid. Values provided: %s", field, vals)
		return errors.New(m)
	}

	min := NoLimit
	max := NoLimit

	groups := vb.strLenRegex.FindStringSubmatch(vals)

	if groups[1] != "" {
		min, _ = strconv.Atoi(groups[1])
	}

	if groups[2] != "" {
		max, _ = strconv.Atoi(groups[2])
	}

	if opParams == 2 {
		sv.Length(min, max)
	} else {
		sv.Length(min, max, ops[2])
	}

	return nil

}

func newStringValidatorBuilder(defaultErrorCode string) *stringValidatorBuilder {
	vb := new(stringValidatorBuilder)
	vb.strLenRegex = regexp.MustCompile("^(\\d*)-(\\d*)$")
	vb.defaultErrorCode = defaultErrorCode

	return vb
}
