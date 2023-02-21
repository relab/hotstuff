// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v3.19.4
// source: internal/proto/kauripb/kauri.proto

package kauripb

import (
	_ "github.com/relab/gorums"
	hotstuffpb "github.com/relab/hotstuff/internal/proto/hotstuffpb"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Contribution struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ID        uint32                      `protobuf:"varint,1,opt,name=ID,proto3" json:"ID,omitempty"`
	Signature *hotstuffpb.QuorumSignature `protobuf:"bytes,2,opt,name=Signature,proto3" json:"Signature,omitempty"`
	View      uint64                      `protobuf:"varint,3,opt,name=View,proto3" json:"View,omitempty"`
}

func (x *Contribution) Reset() {
	*x = Contribution{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_proto_kauripb_kauri_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Contribution) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Contribution) ProtoMessage() {}

func (x *Contribution) ProtoReflect() protoreflect.Message {
	mi := &file_internal_proto_kauripb_kauri_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Contribution.ProtoReflect.Descriptor instead.
func (*Contribution) Descriptor() ([]byte, []int) {
	return file_internal_proto_kauripb_kauri_proto_rawDescGZIP(), []int{0}
}

func (x *Contribution) GetID() uint32 {
	if x != nil {
		return x.ID
	}
	return 0
}

func (x *Contribution) GetSignature() *hotstuffpb.QuorumSignature {
	if x != nil {
		return x.Signature
	}
	return nil
}

func (x *Contribution) GetView() uint64 {
	if x != nil {
		return x.View
	}
	return 0
}

var File_internal_proto_kauripb_kauri_proto protoreflect.FileDescriptor

var file_internal_proto_kauripb_kauri_proto_rawDesc = []byte{
	0x0a, 0x22, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x6b, 0x61, 0x75, 0x72, 0x69, 0x70, 0x62, 0x2f, 0x6b, 0x61, 0x75, 0x72, 0x69, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x07, 0x6b, 0x61, 0x75, 0x72, 0x69, 0x70, 0x62, 0x1a, 0x0c, 0x67,
	0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1b, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70,
	0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x19, 0x68, 0x6f, 0x74, 0x73, 0x74, 0x75,
	0x66, 0x66, 0x70, 0x62, 0x2f, 0x68, 0x6f, 0x74, 0x73, 0x74, 0x75, 0x66, 0x66, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x22, 0x6d, 0x0a, 0x0c, 0x43, 0x6f, 0x6e, 0x74, 0x72, 0x69, 0x62, 0x75, 0x74,
	0x69, 0x6f, 0x6e, 0x12, 0x0e, 0x0a, 0x02, 0x49, 0x44, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52,
	0x02, 0x49, 0x44, 0x12, 0x39, 0x0a, 0x09, 0x53, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x68, 0x6f, 0x74, 0x73, 0x74, 0x75, 0x66,
	0x66, 0x70, 0x62, 0x2e, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x53, 0x69, 0x67, 0x6e, 0x61, 0x74,
	0x75, 0x72, 0x65, 0x52, 0x09, 0x53, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x12, 0x12,
	0x0a, 0x04, 0x56, 0x69, 0x65, 0x77, 0x18, 0x03, 0x20, 0x01, 0x28, 0x04, 0x52, 0x04, 0x56, 0x69,
	0x65, 0x77, 0x32, 0x50, 0x0a, 0x05, 0x4b, 0x61, 0x75, 0x72, 0x69, 0x12, 0x47, 0x0a, 0x10, 0x53,
	0x65, 0x6e, 0x64, 0x43, 0x6f, 0x6e, 0x74, 0x72, 0x69, 0x62, 0x75, 0x74, 0x69, 0x6f, 0x6e, 0x12,
	0x15, 0x2e, 0x6b, 0x61, 0x75, 0x72, 0x69, 0x70, 0x62, 0x2e, 0x43, 0x6f, 0x6e, 0x74, 0x72, 0x69,
	0x62, 0x75, 0x74, 0x69, 0x6f, 0x6e, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04,
	0x90, 0xb5, 0x18, 0x01, 0x42, 0x32, 0x5a, 0x30, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63,
	0x6f, 0x6d, 0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f, 0x68, 0x6f, 0x74, 0x73, 0x74, 0x75, 0x66,
	0x66, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x6b, 0x61, 0x75, 0x72, 0x69, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_internal_proto_kauripb_kauri_proto_rawDescOnce sync.Once
	file_internal_proto_kauripb_kauri_proto_rawDescData = file_internal_proto_kauripb_kauri_proto_rawDesc
)

func file_internal_proto_kauripb_kauri_proto_rawDescGZIP() []byte {
	file_internal_proto_kauripb_kauri_proto_rawDescOnce.Do(func() {
		file_internal_proto_kauripb_kauri_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_proto_kauripb_kauri_proto_rawDescData)
	})
	return file_internal_proto_kauripb_kauri_proto_rawDescData
}

var file_internal_proto_kauripb_kauri_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_internal_proto_kauripb_kauri_proto_goTypes = []interface{}{
	(*Contribution)(nil),               // 0: kauripb.Contribution
	(*hotstuffpb.QuorumSignature)(nil), // 1: hotstuffpb.QuorumSignature
	(*emptypb.Empty)(nil),              // 2: google.protobuf.Empty
}
var file_internal_proto_kauripb_kauri_proto_depIdxs = []int32{
	1, // 0: kauripb.Contribution.Signature:type_name -> hotstuffpb.QuorumSignature
	0, // 1: kauripb.Kauri.SendContribution:input_type -> kauripb.Contribution
	2, // 2: kauripb.Kauri.SendContribution:output_type -> google.protobuf.Empty
	2, // [2:3] is the sub-list for method output_type
	1, // [1:2] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_internal_proto_kauripb_kauri_proto_init() }
func file_internal_proto_kauripb_kauri_proto_init() {
	if File_internal_proto_kauripb_kauri_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_internal_proto_kauripb_kauri_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Contribution); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_internal_proto_kauripb_kauri_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_internal_proto_kauripb_kauri_proto_goTypes,
		DependencyIndexes: file_internal_proto_kauripb_kauri_proto_depIdxs,
		MessageInfos:      file_internal_proto_kauripb_kauri_proto_msgTypes,
	}.Build()
	File_internal_proto_kauripb_kauri_proto = out.File
	file_internal_proto_kauripb_kauri_proto_rawDesc = nil
	file_internal_proto_kauripb_kauri_proto_goTypes = nil
	file_internal_proto_kauripb_kauri_proto_depIdxs = nil
}
