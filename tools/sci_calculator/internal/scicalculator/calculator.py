import ast
import json
import math
import operator
import re
import statistics
import sys
from typing import Any, Dict, List, Optional, Set, Tuple

_numpy = None
_sympy = None
_scipy_stats = None
_scipy_quad = None


ALLOWED_OPERATORS = {
    ast.Add: operator.add,
    ast.Sub: operator.sub,
    ast.Mult: operator.mul,
    ast.Div: operator.truediv,
    ast.FloorDiv: operator.floordiv,
    ast.Mod: operator.mod,
    ast.Pow: operator.pow,
    ast.USub: operator.neg,
    ast.UAdd: operator.pos,
}

MATH_FUNCTIONS = {
    "sin": math.sin,
    "cos": math.cos,
    "tan": math.tan,
    "asin": math.asin,
    "acos": math.acos,
    "atan": math.atan,
    "arcsin": math.asin,
    "arccos": math.acos,
    "arctan": math.atan,
    "sinh": math.sinh,
    "cosh": math.cosh,
    "tanh": math.tanh,
    "asinh": math.asinh,
    "acosh": math.acosh,
    "atanh": math.atanh,
    "sqrt": math.sqrt,
    "log": math.log,
    "exp": math.exp,
    "abs": math.fabs,
    "ceil": math.ceil,
    "floor": math.floor,
    "mean": statistics.mean,
    "median": statistics.median,
    "mode": statistics.mode,
    "variance": statistics.variance,
    "stdev": statistics.stdev,
}

CONSTANTS = {"pi": math.pi, "e": math.e}


def numpy_module() -> Any:
    global _numpy
    if _numpy is None:
        import numpy

        _numpy = numpy
    return _numpy


def sympy_module() -> Any:
    global _sympy
    if _sympy is None:
        import sympy

        _sympy = sympy
    return _sympy


def scipy_stats_module() -> Any:
    global _scipy_stats
    if _scipy_stats is None:
        from scipy import stats

        _scipy_stats = stats
    return _scipy_stats


def scipy_quad_function() -> Any:
    global _scipy_quad
    if _scipy_quad is None:
        from scipy.integrate import quad

        _scipy_quad = quad
    return _scipy_quad


def sympy_functions() -> Dict[str, Any]:
    sp = sympy_module()
    return {
        "sin": sp.sin,
        "cos": sp.cos,
        "tan": sp.tan,
        "asin": sp.asin,
        "acos": sp.acos,
        "atan": sp.atan,
        "atan2": sp.atan2,
        "arcsin": sp.asin,
        "arccos": sp.acos,
        "arctan": sp.atan,
        "sinh": sp.sinh,
        "cosh": sp.cosh,
        "tanh": sp.tanh,
        "asinh": sp.asinh,
        "acosh": sp.acosh,
        "atanh": sp.atanh,
        "sqrt": sp.sqrt,
        "exp": sp.exp,
        "log": sp.log,
        "abs": sp.Abs,
        "Abs": sp.Abs,
        "gamma": sp.gamma,
        "factorial": sp.factorial,
        "Min": sp.Min,
        "Max": sp.Max,
        "DiracDelta": sp.DiracDelta,
        "Heaviside": sp.Heaviside,
    }


def sympy_constants() -> Dict[str, Any]:
    sp = sympy_module()
    return {
        "pi": sp.pi,
        "e": sp.E,
        "E": sp.E,
        "I": sp.I,
        "oo": sp.oo,
        "inf": sp.oo,
        "infinity": sp.oo,
        "nan": sp.nan,
        "zoo": sp.zoo,
    }


def preprocess_expression(expr: str) -> str:
    return str(expr).strip().replace("^", "**")


def parse_symbolic_expression(expr: str, allowed_symbols: Optional[Set[str]] = None) -> Any:
    parsed = ast.parse(preprocess_expression(expr), mode="eval")
    return symbolic_node(parsed.body, allowed_symbols)


def symbolic_node(node: ast.AST, allowed_symbols: Optional[Set[str]] = None) -> Any:
    sp = sympy_module()
    if isinstance(node, ast.Constant):
        if isinstance(node.value, (int, float, complex)):
            return sp.sympify(node.value)
        raise ValueError("symbolic expressions only support numeric constants")
    if isinstance(node, ast.Name):
        constants = sympy_constants()
        if node.id in constants:
            return constants[node.id]
        if allowed_symbols is None or node.id in allowed_symbols:
            return sp.Symbol(node.id)
        raise ValueError(f"unsupported symbolic variable: {node.id}")
    if isinstance(node, ast.BinOp):
        left = symbolic_node(node.left, allowed_symbols)
        right = symbolic_node(node.right, allowed_symbols)
        if isinstance(node.op, ast.Add):
            return left + right
        if isinstance(node.op, ast.Sub):
            return left - right
        if isinstance(node.op, ast.Mult):
            return left * right
        if isinstance(node.op, ast.Div):
            return left / right
        if isinstance(node.op, ast.FloorDiv):
            return sp.floor(left / right)
        if isinstance(node.op, ast.Mod):
            return sp.Mod(left, right)
        if isinstance(node.op, ast.Pow):
            return left**right
        raise ValueError(f"unsupported symbolic operator: {type(node.op).__name__}")
    if isinstance(node, ast.UnaryOp):
        operand = symbolic_node(node.operand, allowed_symbols)
        if isinstance(node.op, ast.USub):
            return -operand
        if isinstance(node.op, ast.UAdd):
            return operand
        raise ValueError(f"unsupported symbolic unary operator: {type(node.op).__name__}")
    if isinstance(node, ast.Call) and isinstance(node.func, ast.Name):
        name = node.func.id
        args = [symbolic_node(arg, allowed_symbols) for arg in node.args]
        if name == "root":
            if len(args) != 2:
                raise ValueError("root() requires x and n")
            return args[0] ** (sp.Integer(1) / args[1])
        function = sympy_functions().get(name)
        if function is None:
            raise ValueError(f"unsupported symbolic function: {name}")
        return function(*args)
    raise ValueError(f"unsupported symbolic expression node: {type(node).__name__}")


def evaluate(expression: str) -> str:
    def eval_expr(node: ast.AST) -> Any:
        if isinstance(node, ast.Constant):
            return node.value
        if isinstance(node, ast.Name):
            if node.id in CONSTANTS:
                return CONSTANTS[node.id]
            name = node.id.lower()
            if name in ("inf", "infinity"):
                return float("inf")
            if name == "nan":
                return float("nan")
            raise ValueError(f"Unsupported variable or constant: {node.id}")
        if isinstance(node, ast.BinOp):
            left = eval_expr(node.left)
            right = eval_expr(node.right)
            op_type = type(node.op)
            if op_type not in ALLOWED_OPERATORS:
                raise ValueError(f"Unsupported binary operation: {op_type.__name__}")
            return ALLOWED_OPERATORS[op_type](left, right)
        if isinstance(node, ast.UnaryOp):
            operand = eval_expr(node.operand)
            op_type = type(node.op)
            if op_type not in ALLOWED_OPERATORS:
                raise ValueError(f"Unsupported unary operation: {op_type.__name__}")
            return ALLOWED_OPERATORS[op_type](operand)
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Name):
            return eval_call(node)
        if isinstance(node, ast.Compare):
            return eval_compare(node)
        if isinstance(node, ast.List):
            return [eval_expr(elt) for elt in node.elts]
        if isinstance(node, ast.Tuple):
            return tuple(eval_expr(elt) for elt in node.elts)
        if isinstance(node, ast.Dict):
            keys = [eval_expr(key) for key in node.keys]
            values = [eval_expr(value) for value in node.values]
            for key in keys:
                if not isinstance(key, (str, int, float)):
                    raise ValueError("Dictionary keys must be strings, integers, or floats")
            return dict(zip(keys, values))
        raise ValueError(f"Unsupported AST node: {type(node).__name__}")

    def eval_call(node: ast.Call) -> Any:
        func_name = node.func.id
        if func_name == "integral":
            return eval_integral_call(node)
        args = [eval_expr(arg) for arg in node.args]
        if func_name == "error_propagation":
            if len(args) != 2 or not isinstance(args[0], str) or not isinstance(args[1], dict):
                raise ValueError("error_propagation() requires expr_str and {var: (value, error)}")
            return compute_error_propagation(args[0], args[1])
        if func_name == "confidence_interval":
            if len(args) not in (2, 3):
                raise ValueError("confidence_interval() requires data_list and confidence_level")
            population_mean = args[2] if len(args) == 3 else None
            return compute_confidence_interval(args[0], args[1], population_mean)
        if func_name == "root":
            if len(args) != 2:
                raise ValueError("root() requires x and n")
            return args[0] ** (1 / args[1])
        if func_name == "norm_pdf":
            if len(args) != 3:
                raise ValueError("norm_pdf() requires x, mean, and std")
            stats = scipy_stats_module()
            return stats.norm.pdf(args[0], loc=args[1], scale=args[2])
        if func_name == "norm_cdf":
            if len(args) != 3:
                raise ValueError("norm_cdf() requires x, mean, and std")
            stats = scipy_stats_module()
            return stats.norm.cdf(args[0], loc=args[1], scale=args[2])
        if func_name == "t_test":
            if len(args) != 2 or not isinstance(args[0], list) or not isinstance(args[1], (int, float)):
                raise ValueError("t_test() requires data_list and mu")
            stats = scipy_stats_module()
            return stats.ttest_1samp(args[0], args[1]).pvalue
        function = MATH_FUNCTIONS.get(func_name)
        if function is None:
            raise ValueError(f"Unsupported function: {func_name}")
        if func_name in {"mean", "median", "mode", "variance", "stdev"} and (len(args) != 1 or not isinstance(args[0], list)):
            raise ValueError(f"{func_name}() requires one list argument")
        return function(*args)

    def eval_integral_call(node: ast.Call) -> Any:
        count = len(node.args)
        if count not in (1, 2, 3, 4):
            raise ValueError("integral() syntax: integral('expr' [, 'var'] [, lower, upper])")
        expression_node = node.args[0]
        if not isinstance(expression_node, ast.Constant) or not isinstance(expression_node.value, str):
            raise ValueError("First argument to integral() must be a string expression")
        expr = expression_node.value
        var_name = "x"
        lower = None
        upper = None
        if count == 2:
            var_name_value = eval_expr(node.args[1])
            if not isinstance(var_name_value, str):
                raise ValueError("Second argument for indefinite integral must be a variable name string")
            var_name = var_name_value
        elif count == 3:
            lower = eval_expr(node.args[1])
            upper = eval_expr(node.args[2])
        elif count == 4:
            var_name_value = eval_expr(node.args[1])
            if not isinstance(var_name_value, str):
                raise ValueError("Second argument for definite integral must be a variable name string")
            var_name = var_name_value
            lower = eval_expr(node.args[2])
            upper = eval_expr(node.args[3])
        return compute_integral(expr, var_name, lower, upper)

    def eval_compare(node: ast.Compare) -> str:
        sp = sympy_module()
        if len(node.ops) > 1:
            raise ValueError("Chained comparisons are not supported")
        left = sp.N(eval_expr(node.left))
        right = sp.N(eval_expr(node.comparators[0]))
        difference = sp.N(left - right)
        if not difference.is_number:
            raise ValueError("Comparison difference is not numeric")
        if difference.is_complex and not difference.is_extended_real:
            _, imaginary = difference.as_real_imag()
            if not math.isclose(float(sp.N(imaginary)), 0, abs_tol=1e-9):
                return f"Cannot compare complex values: $${sp.latex(difference)}$$"
            difference = difference.as_real_imag()[0]
        numeric_difference = float(difference)
        if math.isclose(numeric_difference, 0, abs_tol=1e-9):
            return "Both expressions are equal."
        if numeric_difference > 0:
            return "The left expression is larger."
        return "The right expression is larger."

    try:
        prepared = preprocess_expression(expression)
        if not prepared:
            raise ValueError("Expression cannot be empty")
        validate_brackets(prepared)
        parsed = ast.parse(prepared, mode="eval")
        return format_value(eval_expr(parsed.body))
    except SyntaxError as err:
        return f"Syntax Error: Invalid mathematical expression. Details: {err}"
    except ZeroDivisionError:
        return "Error: Division by zero."
    except OverflowError:
        return "Error: Numerical result out of range."
    except ValueError as err:
        return f"Input Error: {err}"
    except Exception as err:
        return f"Calculation Error: {type(err).__name__}: {err}"


def compute_integral(expr_text: str, var_name: str, lower: Any, upper: Any) -> Any:
    try:
        sp = sympy_module()
        symbol = sp.Symbol(var_name)
        expr = parse_symbolic_expression(expr_text, {var_name})
        if lower is None and upper is None:
            return f"$$ {sp.latex(sp.integrate(expr, symbol))} + C $$"
        lower_limit = symbolic_limit(lower)
        upper_limit = symbolic_limit(upper)
        symbolic_result = sp.integrate(expr, (symbol, lower_limit, upper_limit))
        evaluated = sp.N(symbolic_result, chop=True)
        if should_use_numerical_integral(symbolic_result, evaluated):
            return numerical_integral(expr, symbol, lower_limit, upper_limit, symbolic_result, evaluated)
        if evaluated.is_extended_real and evaluated.is_finite:
            return float(evaluated)
        if evaluated.is_complex and evaluated.is_finite:
            return f"$${sp.latex(evaluated)}$$"
        return f"Symbolic result: $${sp.latex(symbolic_result)}$$ evaluated to $${sp.latex(evaluated)}$$"
    except Exception as err:
        return f"Error: Integral computation failed: {type(err).__name__}: {err}"


def symbolic_limit(value: Any) -> Any:
    sp = sympy_module()
    if isinstance(value, str):
        lowered = value.strip().lower()
        if lowered in ("inf", "infinity"):
            return sp.oo
        if lowered in ("-inf", "-infinity"):
            return -sp.oo
        return parse_symbolic_expression(value, set())
    if isinstance(value, (int, float)):
        if math.isinf(value):
            return sp.oo if value > 0 else -sp.oo
        if math.isnan(value):
            return sp.nan
        return sp.sympify(value)
    return sp.sympify(value)


def should_use_numerical_integral(symbolic_result: Any, evaluated: Any) -> bool:
    sp = sympy_module()
    if isinstance(symbolic_result, sp.Integral):
        return True
    return bool(evaluated.has(sp.oo, -sp.oo, sp.zoo, sp.nan))


def numerical_integral(expr: Any, symbol: Any, lower: Any, upper: Any, symbolic_result: Any, evaluated: Any) -> Any:
    sp = sympy_module()
    prefix = f"Symbolic result ($${sp.latex(symbolic_result)}$$) evaluated to ($${sp.latex(evaluated)}$$). "
    if isinstance(symbolic_result, sp.Integral):
        prefix = f"Symbolic integration unevaluated ($${sp.latex(symbolic_result)}$$). "
    try:
        q_lower = limit_to_float(lower)
        q_upper = limit_to_float(upper)
    except ValueError as err:
        return prefix + f"Numerical integration failed: {err}"
    if q_lower >= q_upper and not (math.isinf(q_lower) and math.isinf(q_upper) and q_lower == q_upper):
        return prefix + f"Numerical integration error: lower limit {q_lower} must be less than upper limit {q_upper}."
    try:
        quad = scipy_quad_function()
        value, error = quad(lambda x: numerical_integrand(expr, symbol, x), q_lower, q_upper, limit=150, epsabs=1.49e-7, epsrel=1.49e-7)
    except Exception as err:
        return prefix + f"Numerical integration failed: {type(err).__name__}: {err}"
    if math.isnan(value):
        return prefix + "Numerical integration resulted in NaN."
    if abs(error) > 0.01 * abs(value) and abs(error) > 1e-4:
        return prefix + f"Numerical result: {value:.7g} (Warning: potentially large error: {error:.2g})"
    return float(value)


def limit_to_float(value: Any) -> float:
    sp = sympy_module()
    if value == sp.oo:
        return math.inf
    if value == -sp.oo:
        return -math.inf
    evaluated = sp.N(value)
    if evaluated.has(sp.nan, sp.zoo):
        raise ValueError(f"limit is not numeric: {sp.latex(value)}")
    if evaluated.is_infinite:
        return math.inf if evaluated.is_extended_positive else -math.inf
    if not evaluated.is_number:
        raise ValueError(f"limit is not numeric: {sp.latex(value)}")
    return float(evaluated)


def numerical_integrand(expr: Any, symbol: Any, value: float) -> float:
    sp = sympy_module()
    np = numpy_module()
    evaluated = sp.N(expr.subs({symbol: value}), chop=True)
    if evaluated in (sp.nan, sp.zoo):
        return np.nan
    if evaluated == sp.oo:
        return np.inf
    if evaluated == -sp.oo:
        return -np.inf
    if not evaluated.is_extended_real:
        real, imaginary = evaluated.as_real_imag()
        if not math.isclose(float(sp.N(imaginary)), 0, abs_tol=1e-9):
            return np.nan
        evaluated = real
    if evaluated.is_infinite:
        return np.inf if evaluated.is_extended_positive else -np.inf
    return float(evaluated)


def compute_error_propagation(expr_text: str, variables: Dict[str, Tuple[float, float]]) -> str:
    sp = sympy_module()
    symbol_names = set(variables.keys())
    expr = parse_symbolic_expression(expr_text, symbol_names)
    symbols = {name: sp.Symbol(name) for name in symbol_names}
    substitutions = {}
    total_error_sq = sp.S.Zero
    for name, pair in variables.items():
        if not isinstance(pair, tuple) or len(pair) != 2:
            raise ValueError("error_propagation variable values must be (value, error) tuples")
        value, error = pair
        if not isinstance(value, (int, float)) or not isinstance(error, (int, float)):
            raise ValueError("error_propagation values and errors must be numbers")
        substitutions[symbols[name]] = value
    calculated = sp.N(expr.subs(substitutions))
    for name, pair in variables.items():
        _, error = pair
        derivative = sp.N(sp.diff(expr, symbols[name]).subs(substitutions))
        if not derivative.is_number:
            raise ValueError(f"partial derivative for {name} is not numeric")
        total_error_sq += (derivative * error) ** 2
    final_error = sp.N(sp.sqrt(total_error_sq))
    if not calculated.is_number or not final_error.is_number:
        raise ValueError("calculated value or propagated error is not numeric")
    return f"Value = {float(calculated):.7g}, Error = {float(final_error):.4g}"


def compute_confidence_interval(data: List[Any], confidence: float, population_mean: Optional[float] = None) -> str:
    if population_mean is not None:
        _ = population_mean
    if not isinstance(data, list) or not all(isinstance(item, (int, float)) for item in data):
        raise ValueError("confidence_interval() data must be a list of numbers")
    if not isinstance(confidence, (int, float)) or not 0 < confidence < 1:
        raise ValueError("confidence level must be a number between 0 and 1")
    if len(data) < 2:
        raise ValueError("confidence_interval() needs at least 2 data points")
    mean = statistics.mean(data)
    std_dev = statistics.stdev(data)
    stats = scipy_stats_module()
    lower, upper = stats.t.interval(confidence, df=len(data) - 1, loc=mean, scale=std_dev / math.sqrt(len(data)))
    return f"[{float(lower):.6g}, {float(upper):.6g}] ({confidence * 100:.0f}% CI for mean)"


def validate_brackets(expression: str) -> None:
    brackets = {"(": ")", "[": "]", "{": "}"}
    stack: List[str] = []
    in_string = False
    string_char = ""
    escaped = False
    for char in expression:
        if in_string:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == string_char:
                in_string = False
            continue
        if char in "'\"":
            in_string = True
            string_char = char
        elif char in brackets:
            stack.append(char)
        elif char in brackets.values():
            if not stack or brackets[stack.pop()] != char:
                raise SyntaxError("mismatched parentheses or brackets")
    if stack:
        raise SyntaxError("unclosed parentheses or brackets")


def split_expressions(text: str) -> List[str]:
    parts: List[str] = []
    nesting = 0
    start = 0
    in_string = False
    string_char = ""
    escaped = False
    for index, char in enumerate(text):
        if in_string:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == string_char:
                in_string = False
            continue
        if char in "'\"":
            in_string = True
            string_char = char
        elif char in "([{":
            nesting += 1
        elif char in ")]}":
            nesting -= 1
        elif char == "," and nesting == 0:
            parts.append(text[start:index].strip())
            start = index + 1
    parts.append(text[start:].strip())
    return [part for part in parts if part]


def expressions_from_input(raw_input: str) -> List[str]:
    try:
        payload = json.loads(raw_input)
        if isinstance(payload, dict) and isinstance(payload.get("expression"), str):
            raw_input = payload["expression"]
    except json.JSONDecodeError:
        pass
    try:
        literal = ast.literal_eval(raw_input)
        if isinstance(literal, dict):
            keys = sorted(key for key in literal if isinstance(key, str) and key.startswith("expression"))
            values = [literal[key] for key in keys if isinstance(literal[key], str)]
            if values:
                return values
    except (ValueError, SyntaxError):
        pass
    return split_expressions(raw_input)


def format_value(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, int):
        return str(value)
    numeric_types = [float]
    if _numpy is not None:
        numeric_types.append(_numpy.floating)
    if _sympy is not None:
        if isinstance(value, _sympy.Integer):
            return str(value)
        numeric_types.extend([_sympy.Float, _sympy.Rational, _sympy.Number])
    if isinstance(value, tuple(numeric_types)):
        try:
            number = float(value)
            if math.isinf(number) or math.isnan(number):
                return str(number)
            formatted = f"{number:.10g}"
            if "." in formatted:
                formatted = formatted.rstrip("0").rstrip(".")
            return formatted
        except Exception:
            return str(value)
    if isinstance(value, complex):
        return f"{value.real:.10g}{'+' if value.imag >= 0 else ''}{value.imag:.10g}j".replace("+-", "-")
    return str(value)


def main() -> None:
    raw_input = sys.stdin.readline().strip()
    if not raw_input:
        print(json.dumps({"status": "error", "error": "SciCalculator Plugin Error: No expression provided."}))
        sys.exit(1)
    expressions = expressions_from_input(raw_input)
    cleaned = [re.sub(r"^\s*\d+[\.)]\s*", "", item).strip() for item in expressions]
    results = [evaluate(item) for item in cleaned if item]
    error_prefixes = ("Error:", "Syntax Error:", "Input Error:", "Calculation Error:")
    has_error = any(any(str(result).startswith(prefix) for prefix in error_prefixes) for result in results)
    if len(results) > 1:
        result_text = ",\n".join(f"{original.strip()} = {result}" for original, result in zip(expressions, results))
    elif results:
        result_text = str(results[0])
    else:
        result_text = "No valid expressions found to evaluate."
        has_error = True
    if has_error:
        print(json.dumps({"status": "error", "error": result_text}))
        sys.exit(1)
    if "\n" in result_text:
        result_text = f"###计算结果：\n{result_text}\n###，请将结果转告用户"
    else:
        result_text = f"###计算结果：{result_text}###，请将结果转告用户"
    print(json.dumps({"status": "success", "result": result_text}))
    sys.exit(0)


if __name__ == "__main__":
    main()
