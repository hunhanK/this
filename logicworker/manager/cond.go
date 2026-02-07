/**
 * @Author: PengZiMing
 * @Desc:
 * @Date: 2021/10/15 14:52
 */

package manager

import (
	"fmt"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"strings"
)

type condValueFunc func(player iface.IPlayer, args ...any) uint32

var chckeMap = map[string]condValueFunc{
	"MergeTime": func(player iface.IPlayer, args ...any) uint32 {
		return gshare.GetMergeTimes()
	},

	"MergeDay": func(player iface.IPlayer, args ...any) uint32 {
		return gshare.GetMergeSrvDay()
	},

	"OpenDay": func(player iface.IPlayer, args ...any) uint32 {
		return gshare.GetOpenServerDay()
	},

	// arg[0] = 副本id arg[1]=难度
	"MaterialFubenPreDiffGrade": func(player iface.IPlayer, args ...any) uint32 {
		if len(args) < 2 {
			return 0
		}

		fubenId, ok := args[0].(uint32)
		if !ok {
			return 0
		}

		challengeLv, ok := args[1].(uint32)
		if !ok {
			return 0
		}

		binaryData := player.GetBinaryData()

		fubenInfo, ok := binaryData.MaterialFubenData.Data[fubenId]
		if !ok {
			return 0
		}

		foo, ok := fubenInfo.PassInfo[challengeLv-1]
		if !ok {
			return 0
		}
		return foo.Star
	},
	"Level": func(player iface.IPlayer, args ...any) uint32 {
		return player.GetLevel()
	},
	"fairyWingLv": func(player iface.IPlayer, args ...any) uint32 {
		return uint32(player.GetExtraAttr(attrdef.FairyWingLv))
	},
	"magebodyLv": func(player iface.IPlayer, args ...any) uint32 {
		return uint32(player.GetExtraAttr(attrdef.MageBodyLv))
	},
	"fourSymbolsDragonLv": func(player iface.IPlayer, args ...any) uint32 {
		binary := player.GetBinaryData()
		if nil == binary.FourSymbols || nil == binary.FourSymbols[custom_id.FourSymbolsDragon] {
			return 0
		}
		return binary.FourSymbols[custom_id.FourSymbolsDragon].Level
	},
	"fourSymbolsTigerLv": func(player iface.IPlayer, args ...any) uint32 {
		binary := player.GetBinaryData()
		if nil == binary.FourSymbols {
			return 0
		}
		return binary.FourSymbols[custom_id.FourSymbolsTiger].Level
	},
	"fourSymbolsRosefinchLv": func(player iface.IPlayer, args ...any) uint32 {
		binary := player.GetBinaryData()
		if nil == binary.FourSymbols {
			return 0
		}
		return binary.FourSymbols[custom_id.FourSymbolsRosefinch].Level
	},
	"fourSymbolsTortoiseLv": func(player iface.IPlayer, args ...any) uint32 {
		binary := player.GetBinaryData()
		if nil == binary.FourSymbols {
			return 0
		}
		return binary.FourSymbols[custom_id.FourSymbolsTortoise].Level
	},
	"riderLevel": func(player iface.IPlayer, args ...any) uint32 {
		binary := player.GetBinaryData()
		if nil == binary.RiderData {
			return 0
		}

		if binary.RiderData.ExpLv == nil {
			return 0
		}
		return binary.RiderData.ExpLv.Lv
	},
}

type CondChecker struct{}

func (c *CondChecker) cmp_op(op string, left, right uint32) (bool, error) {
	if op == ">" {
		return left > right, nil
	} else if op == "<" {
		return left < right, nil
	} else if op == ">=" {
		return left >= right, nil
	} else if op == "<=" {
		return left <= right, nil
	} else if op == "=" {
		return left == right, nil
	} else if op == "!=" {
		return left != right, nil
	}

	return false, fmt.Errorf("invalid op: %s", op)
}

func (c *CondChecker) Check(player iface.IPlayer, expr string, conf map[string]uint32, args ...any) (ok bool, err error) {
	if len(expr) == 0 {
		return false, fmt.Errorf("invalid expression: %s", expr)
	}

	if !(expr[0] == '[' || expr[0] == '(') {
		return false, fmt.Errorf("invalid expression: %s", expr)
	}

	leftExp, err := c.extractBracket(expr)
	if nil != err {
		return false, fmt.Errorf("extractBracket failed: %s", err.Error())
	}

	switch leftExp[0] {
	case '[':
		ok, err = c.calAtom(player, leftExp, conf, args...)
	case '(':
		ok, err = c.Check(player, leftExp[1:len(leftExp)-1], conf, args...)
	}

	if err != nil {
		return false, err
	}

	restExpr := expr[len(leftExp):]

	for len(restExpr) > 0 {
		logic_op := string(restExpr[1])
		rightExp, err := c.extractBracket(restExpr[3:])
		if err != nil {
			return false, err
		}

		rightOk, err := c.Check(player, rightExp, conf, args...)
		if err != nil {
			return false, err
		}
		ok, err = c.logicOp(ok, logic_op, rightOk)
		if err != nil {
			return false, err
		}

		leftExp = leftExp + restExpr[:len(rightExp)+3]
		restExpr = expr[len(leftExp):]
	}
	return ok, err
}

func (c *CondChecker) logicOp(left bool, op string, right bool) (bool, error) {
	if op == "&" {
		return left && right, nil
	} else if op == "|" {
		return left || right, nil
	}

	return false, fmt.Errorf("invalid op: %s", op)
}

func (c *CondChecker) extractBracket(exp string) (atomExp string, err error) {
	if len(exp) == 0 {
		return exp, fmt.Errorf("invalid expression: %s", exp)
	}

	stack := []rune{}
	frontBracket := '['
	endBracket := ']'
	if exp[0] == '[' {
		frontBracket = '['
		endBracket = ']'
	} else if exp[0] == '(' {
		frontBracket = '('
		endBracket = ')'
	} else {
		return atomExp, fmt.Errorf("invalid expression: %s", exp)
	}

	for idx, ru := range exp {
		if ru == frontBracket {
			stack = append(stack, ru)
		} else if ru == endBracket {
			if len(stack) == 0 {
				return "", fmt.Errorf("invalid expression: %s", exp)
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return exp[:idx+1], nil
			}
		}
	}

	return atomExp, fmt.Errorf("invalid expression: %s", exp)
}

func (c *CondChecker) calAtom(player iface.IPlayer, atomExpr string, conf map[string]uint32, args ...any) (bool, error) {
	if atomExpr[0] != '[' {
		return false, fmt.Errorf("calAtom failed invalid expression: %s", atomExpr)
	}

	sp := strings.Split(atomExpr, " ")
	if len(sp) != 2 {
		return false, fmt.Errorf("calAtom failed invalid expression: %s", atomExpr)
	}
	opfoo := sp[1][:len(sp[1])-1]
	fname := sp[0][1:]

	fn, ok := chckeMap[fname]
	if !ok {
		return false, fmt.Errorf("calAtom failed invalid expression: %s", atomExpr)
	}

	rightVal, ok := conf[fname]
	if !ok {
		return false, fmt.Errorf("calAtom failed get right value failed key: %s", fname)
	}
	return c.cmp_op(opfoo, fn(player, args...), rightVal)
}
