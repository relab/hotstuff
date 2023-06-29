package fuzz

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func depthToTabs(depth int) (tabs string) {
	for i := 0; i < depth; i++ {
		tabs += "\t"
	}
	return
}

// work in progress
func interfaceToString(i interface{}) string {
	return fmt.Sprintf("%v", i)
}

func valueToString(v protoreflect.Value, depth int) string {
	_, ok := v.Interface().(protoreflect.Message)
	if ok {
		return protoToString(v.Message(), depth+1)
	}
	return interfaceToString(v.Interface())
}

func listToString(list protoreflect.List, depth int) string {
	if list.Len() == 0 {
		return "[]"
	}

	tabs := depthToTabs(depth)

	str := "[\n"

	for i := 0; i < list.Len(); i++ {
		item := list.Get(i)
		str += tabs + "\t" + valueToString(item, depth) + ",\n"
	}

	str += tabs + "]"

	return str
}

func mapToString(mp protoreflect.Map, depth int) string {
	if mp.Len() == 0 {
		return "[]"
	}

	tabs := depthToTabs(depth)
	str := "[\n"

	mp.Range(func(mk protoreflect.MapKey, v protoreflect.Value) bool {
		str += tabs + "\t" + valueToString(mk.Value(), depth) + ": " + valueToString(v, depth) + ",\n"
		return true
	})

	str += tabs + "]"
	return str
}

func fieldToString(field protoreflect.FieldDescriptor, prm protoreflect.Message, depth int) string {
	msg := field.Message()

	str := ""

	if msg == nil {
		return str + prm.Get(field).String() + "\n"
	}

	if field.IsList() {
		list := prm.Get(field).List()
		return str + listToString(list, depth) + "\n"
	}

	if field.IsExtension() {
		panic("extension to string not implemented")
	}

	if field.IsMap() {
		map2 := prm.Get(field).Map()
		return str + mapToString(map2, depth) + "\n"
	}

	if field.IsPlaceholder() {
		panic("placeholder to string not implemented")
	}

	refl := prm.Get(field).Message()
	if refl == nil {
		panic("message reflection is nil")
	}
	return str + protoToString(refl, depth) + "\n"
}

func protoToString(prm protoreflect.Message, depth int) string {
	if !prm.IsValid() {
		return "nil\n"
	}

	desc := prm.Descriptor()
	tabs := depthToTabs(depth)
	str := string(desc.FullName()) + "{\n"
	fields := desc.Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		if field.ContainingOneof() != nil {
			continue
		}

		str += tabs + "\t" + string(field.Name()) + ": " + fieldToString(field, prm, depth+1)
	}

	oneofs := desc.Oneofs()

	for i := 0; i < oneofs.Len(); i++ {
		oneof := oneofs.Get(i)

		fields := oneof.Fields()

		for j := 0; j < fields.Len(); j++ {
			field := fields.Get(j)

			refl := prm.Get(field).Message()
			if !refl.IsValid() {
				continue
			}

			str += tabs + "\t" + string(oneof.Name()) + ": " + fieldToString(field, prm, depth+1)
		}
	}

	str += tabs + "}"

	return str
}
