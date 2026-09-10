package config

import (
	"encoding/json"
	"reflect"
	"strconv"
)

// SystemConfigEntry is the narrow data contract needed to apply persisted
// configuration to a Go struct. It keeps this reflection helper independent
// from the database entity that happens to supply the values.
type SystemConfigEntry interface {
	ConfigKey() string
	ConfigValue() string
	ConfigType() string
}

func SystemConfigSliceReflectToStruct[T SystemConfigEntry](slice []T, structType any) {
	v := reflect.ValueOf(structType).Elem()

	for _, config := range slice {
		field := v.FieldByName(config.ConfigKey())
		if field.Kind() == reflect.Ptr && field.Type().Elem().Kind() == reflect.Bool {
			if config.ConfigValue() == "" {
				field.Set(reflect.Zero(field.Type()))
			} else {
				boolValue, _ := strconv.ParseBool(config.ConfigValue())
				field.Set(reflect.ValueOf(&boolValue))
			}
			continue
		}

		if field.IsValid() && field.CanSet() {
			switch config.ConfigType() {
			case "string":
				field.SetString(config.ConfigValue())
			case "bool":
				boolValue, _ := strconv.ParseBool(config.ConfigValue())
				field.SetBool(boolValue)
			case "int":
				intValue, _ := strconv.Atoi(config.ConfigValue())
				field.SetInt(int64(intValue))
			case "int64":
				intValue, _ := strconv.ParseInt(config.ConfigValue(), 10, 64)
				field.SetInt(intValue)
			case "interface":
				_ = json.Unmarshal([]byte(config.ConfigValue()), field.Addr().Interface())
			default:
				break
			}
		}
	}
}
