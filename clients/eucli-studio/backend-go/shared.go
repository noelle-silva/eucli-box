//go:build ignore

// shared.go — reference map of shared utility functions in the main package.
//
// This file is not compiled; it serves as a cross-reference for developers
// to quickly locate utility function definitions across the codebase.
//
// When adding new shared utilities, update this map to keep it accurate.

package main

// Shared utility function index:
//
//	main.go
//	 638  cleanStorageKey
//	 683  cleanImageRelPath
//	 705  safeJoin
//	 722  atomicWriteFile
//	 764  isAllowedImageExt
//	 887  firstNonEmptyString
//	 897  truthy
//	 913  asString
//	 926  asInt64
//	 945  nowMs
//
//	ai_request_builder.go
//	 472  normalizeObjectList
//	 576  asMap
//	 581  asSlice
//	 586  asFloat64
//	 605  clampInt64
//
//	ai_storage_patch.go
//	 277  objectListAsAny
//
//	split_indexes.go
//	 139  normalizeStringMapGo
//	 152  normalizeStringIDsKeepOrder
//	 167  asMapOrNew
