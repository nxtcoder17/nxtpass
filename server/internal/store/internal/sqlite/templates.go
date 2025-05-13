package sqlite

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

func flattenNumber[T any](n ...T) string {
	result := make([]byte, 0, len(n)*4)
	for i := range n {
		result = fmt.Appendf(result, "%v", n[i])
		if i != len(n)-1 {
			result = append(result, ',')
		}
	}

	return string(result)
}

func SQLParse(s string) *template.Template {
	t := template.New("sample")
	t.Funcs(sprig.FuncMap())
	t.Funcs(template.FuncMap{
		"flatten": func(v any) string {
			switch val := v.(type) {
			case []string:
				{
					result := make([]byte, 0, 20)
					for i := range val {
						result = append(result, '\'')
						result = append(result, val[i]...)
						result = append(result, '\'')
						if i != len(val)-1 {
							result = append(result, ',')
						}
					}
					return string(result)
				}
			case []int:
				return flattenNumber(val...)
			case []int32:
				return flattenNumber(val...)
			case []int64:
				return flattenNumber(val...)
			case []float32:
				return flattenNumber(val...)
			case []float64:
				return flattenNumber(val...)
			case map[string]string:
				{
					result := make([]byte, 0, 20)
					for k, v := range val {
						result = fmt.Appendf(result, "'%s'", k)
						result = fmt.Appendf(result, ",")
						result = fmt.Appendf(result, "'%s'", v)
					}
					return string(result)
				}
			default:
				return fmt.Sprintf("%#v", v)
			}
		},
	})

	t, err := t.Parse(s)
	if err != nil {
		panic(err)
	}

	return t
}

func SQLPrepare(t *template.Template, values any) (string, error) {
	var b bytes.Buffer
	if err := t.Execute(&b, values); err != nil {
		return "", err
	}

	return b.String(), nil
}
