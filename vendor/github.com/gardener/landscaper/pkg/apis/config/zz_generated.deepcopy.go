// +build !ignore_autogenerated

/*
Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file

SPDX-License-Identifier: Apache-2.0
*/
// Code generated by deepcopy-gen. DO NOT EDIT.

package config

import (
	v2 "github.com/gardener/component-spec/bindings-go/apis/v2"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LandscaperConfiguration) DeepCopyInto(out *LandscaperConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	if in.RepositoryContext != nil {
		in, out := &in.RepositoryContext, &out.RepositoryContext
		*out = new(v2.RepositoryContext)
		**out = **in
	}
	if in.DefaultOCI != nil {
		in, out := &in.DefaultOCI, &out.DefaultOCI
		*out = new(OCIConfiguration)
		(*in).DeepCopyInto(*out)
	}
	in.Registry.DeepCopyInto(&out.Registry)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LandscaperConfiguration.
func (in *LandscaperConfiguration) DeepCopy() *LandscaperConfiguration {
	if in == nil {
		return nil
	}
	out := new(LandscaperConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *LandscaperConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LocalRegistryConfiguration) DeepCopyInto(out *LocalRegistryConfiguration) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LocalRegistryConfiguration.
func (in *LocalRegistryConfiguration) DeepCopy() *LocalRegistryConfiguration {
	if in == nil {
		return nil
	}
	out := new(LocalRegistryConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OCICacheConfiguration) DeepCopyInto(out *OCICacheConfiguration) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OCICacheConfiguration.
func (in *OCICacheConfiguration) DeepCopy() *OCICacheConfiguration {
	if in == nil {
		return nil
	}
	out := new(OCICacheConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OCIConfiguration) DeepCopyInto(out *OCIConfiguration) {
	*out = *in
	if in.ConfigFiles != nil {
		in, out := &in.ConfigFiles, &out.ConfigFiles
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Cache != nil {
		in, out := &in.Cache, &out.Cache
		*out = new(OCICacheConfiguration)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OCIConfiguration.
func (in *OCIConfiguration) DeepCopy() *OCIConfiguration {
	if in == nil {
		return nil
	}
	out := new(OCIConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RegistryConfiguration) DeepCopyInto(out *RegistryConfiguration) {
	*out = *in
	if in.Local != nil {
		in, out := &in.Local, &out.Local
		*out = new(LocalRegistryConfiguration)
		**out = **in
	}
	if in.OCI != nil {
		in, out := &in.OCI, &out.OCI
		*out = new(OCIConfiguration)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RegistryConfiguration.
func (in *RegistryConfiguration) DeepCopy() *RegistryConfiguration {
	if in == nil {
		return nil
	}
	out := new(RegistryConfiguration)
	in.DeepCopyInto(out)
	return out
}
