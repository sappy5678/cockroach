// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package catalog

import (
	"time"

	"github.com/cockroachdb/cockroach/pkg/geo/geoindex"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/catpb"
	"github.com/cockroachdb/cockroach/pkg/sql/catalog/descpb"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/sql/types"
	"github.com/cockroachdb/cockroach/pkg/util/hlc"
	"github.com/cockroachdb/cockroach/pkg/util/iterutil"
)

// TableElementMaybeMutation is an interface used as a subtype for the various
// table descriptor elements which may be present in a mutation.
type TableElementMaybeMutation interface {
	// IsMutation returns true iff this table element is in a mutation.
	IsMutation() bool

	// IsRollback returns true iff the table element is in a rollback mutation.
	IsRollback() bool

	// MutationID returns the table element's mutationID if applicable,
	// descpb.InvalidMutationID otherwise.
	MutationID() descpb.MutationID

	// WriteAndDeleteOnly returns true iff the table element is in a mutation in
	// the delete-and-write-only state.
	WriteAndDeleteOnly() bool

	// DeleteOnly returns true iff the table element is in a mutation in the
	// delete-only state.
	DeleteOnly() bool

	// Backfilling returns true iff the table element is in a
	// mutation in the backfilling state.
	Backfilling() bool

	// Mergin returns true iff the table element is in a
	// mutation in the merging state.
	Merging() bool

	// Adding returns true iff the table element is in an add mutation.
	Adding() bool

	// Dropped returns true iff the table element is in a drop mutation.
	Dropped() bool
}

// Mutation is an interface around a table descriptor mutation.
type Mutation interface {
	TableElementMaybeMutation

	// AsColumn returns the corresponding Column if the mutation is on a column,
	// nil otherwise.
	AsColumn() Column

	// AsIndex returns the corresponding Index if the mutation is on an index,
	// nil otherwise.
	AsIndex() Index

	// AsConstraint returns the corresponding ConstraintToUpdate if the mutation
	// is on a constraint, nil otherwise.
	AsConstraint() ConstraintToUpdate

	// AsPrimaryKeySwap returns the corresponding PrimaryKeySwap if the mutation
	// is a primary key swap, nil otherwise.
	AsPrimaryKeySwap() PrimaryKeySwap

	// AsComputedColumnSwap returns the corresponding ComputedColumnSwap if the
	// mutation is a computed column swap, nil otherwise.
	AsComputedColumnSwap() ComputedColumnSwap

	// AsMaterializedViewRefresh returns the corresponding MaterializedViewRefresh
	// if the mutation is a materialized view refresh, nil otherwise.
	AsMaterializedViewRefresh() MaterializedViewRefresh

	// NOTE: When adding new types of mutations to this interface, be sure to
	// audit the code which unpacks and introspects mutations to be sure to add
	// cases for the new type.

	// MutationOrdinal returns the ordinal of the mutation in the underlying table
	// descriptor's Mutations slice.
	MutationOrdinal() int
}

// Index is an interface around the index descriptor types.
type Index interface {
	TableElementMaybeMutation

	// IndexDesc returns the underlying protobuf descriptor.
	// Ideally, this method should be called as rarely as possible.
	IndexDesc() *descpb.IndexDescriptor

	// IndexDescDeepCopy returns a deep copy of the underlying proto.
	IndexDescDeepCopy() descpb.IndexDescriptor

	// Ordinal returns the ordinal of the index in its parent table descriptor.
	//
	// The ordinal of an index in a `tableDesc descpb.TableDescriptor` is
	// defined as follows:
	// - 0 is the ordinal of the primary index,
	// - [1:1+len(tableDesc.Indexes)] is the range of public non-primary indexes,
	// - [1+len(tableDesc.Indexes):] is the range of non-public indexes.
	//
	// In terms of a `table catalog.TableDescriptor` interface, it is defined
	// as the catalog.Index object's position in the table.AllIndexes() slice.
	Ordinal() int

	// Primary returns true iff the index is the primary index for the table
	// descriptor.
	Primary() bool

	// Public returns true iff the index is active, i.e. readable, in the table
	// descriptor.
	Public() bool

	// The remaining methods operate on the underlying descpb.IndexDescriptor object.

	GetID() descpb.IndexID
	GetConstraintID() descpb.ConstraintID
	GetName() string
	IsPartial() bool
	IsUnique() bool
	IsDisabled() bool
	IsSharded() bool
	IsCreatedExplicitly() bool
	GetPredicate() string
	GetType() descpb.IndexDescriptor_Type
	GetGeoConfig() geoindex.Config
	GetVersion() descpb.IndexDescriptorVersion
	GetEncodingType() descpb.IndexDescriptorEncodingType

	GetSharded() catpb.ShardedDescriptor
	GetShardColumnName() string

	IsValidOriginIndex(originColIDs descpb.ColumnIDs) bool
	IsValidReferencedUniqueConstraint(referencedColIDs descpb.ColumnIDs) bool

	GetPartitioning() Partitioning

	ExplicitColumnStartIdx() int

	NumKeyColumns() int
	GetKeyColumnID(columnOrdinal int) descpb.ColumnID
	GetKeyColumnName(columnOrdinal int) string
	GetKeyColumnDirection(columnOrdinal int) descpb.IndexDescriptor_Direction

	CollectKeyColumnIDs() TableColSet
	CollectKeySuffixColumnIDs() TableColSet
	CollectPrimaryStoredColumnIDs() TableColSet
	CollectSecondaryStoredColumnIDs() TableColSet
	CollectCompositeColumnIDs() TableColSet

	// InvertedColumnID returns the ColumnID of the inverted column of the
	// inverted index.
	//
	// Panics if the index is not inverted.
	InvertedColumnID() descpb.ColumnID

	// InvertedColumnName returns the name of the inverted column of the inverted
	// index.
	//
	// Panics if the index is not inverted.
	InvertedColumnName() string

	// InvertedColumnKeyType returns the type of the data element that is encoded
	// as the inverted index key. This is currently always Bytes.
	//
	// Panics if the index is not inverted.
	InvertedColumnKeyType() *types.T

	NumPrimaryStoredColumns() int
	NumSecondaryStoredColumns() int
	GetStoredColumnID(storedColumnOrdinal int) descpb.ColumnID
	GetStoredColumnName(storedColumnOrdinal int) string
	HasOldStoredColumns() bool

	NumKeySuffixColumns() int
	GetKeySuffixColumnID(extraColumnOrdinal int) descpb.ColumnID

	NumCompositeColumns() int
	GetCompositeColumnID(compositeColumnOrdinal int) descpb.ColumnID
	UseDeletePreservingEncoding() bool
	// ForcePut, if true, forces all writes to use Put rather than CPut or InitPut.
	//
	// Users of this options should take great care as it
	// effectively mean unique constraints are not respected.
	//
	// Currently (2022-01-19) this two users: delete preserving
	// indexes and merging indexes.
	//
	// Delete preserving encoding indexes are used only as a log of
	// index writes during backfill, thus we can blindly put values into
	// them.
	//
	// New indexes may miss updates during the backfilling process
	// that would lead to CPut failures until the missed updates
	// are merged into the index. Uniqueness for such indexes is
	// checked by the schema changer before they are brought back
	// online.
	ForcePut() bool

	// CreatedAt is an approximate timestamp at which the index was created.
	// It is derived from the statement time at which the relevant statement
	// was issued.
	CreatedAt() time.Time
}

// Column is an interface around the column descriptor types.
type Column interface {
	TableElementMaybeMutation

	// ColumnDesc returns the underlying protobuf descriptor.
	// Ideally, this method should be called as rarely as possible.
	ColumnDesc() *descpb.ColumnDescriptor

	// ColumnDescDeepCopy returns a deep copy of the underlying proto.
	ColumnDescDeepCopy() descpb.ColumnDescriptor

	// DeepCopy returns a deep copy of the receiver.
	DeepCopy() Column

	// Ordinal returns the ordinal of the column in its parent table descriptor.
	//
	// The ordinal of a column in a `tableDesc descpb.TableDescriptor` is
	// defined as follows:
	// - [0:len(tableDesc.Columns)] is the range of public columns,
	// - [len(tableDesc.Columns):] is the range of non-public columns.
	//
	// In terms of a `table catalog.TableDescriptor` interface, it is defined
	// as the catalog.Column object's position in the table.AllColumns() slice.
	Ordinal() int

	// Public returns true iff the column is active, i.e. readable, in the table
	// descriptor.
	Public() bool

	// GetID returns the column ID.
	GetID() descpb.ColumnID

	// GetName returns the column name as a string.
	GetName() string

	// ColName returns the column name as a tree.Name.
	ColName() tree.Name

	// HasType returns true iff the column type is set.
	HasType() bool

	// GetType returns the column type.
	GetType() *types.T

	// IsNullable returns true iff the column allows NULL values.
	IsNullable() bool

	// HasDefault returns true iff the column has a default expression set.
	HasDefault() bool

	// GetDefaultExpr returns the column default expression if it exists,
	// empty string otherwise.
	GetDefaultExpr() string

	// HasOnUpdate returns true iff the column has an on update expression set.
	HasOnUpdate() bool

	// GetOnUpdateExpr returns the column on update expression if it exists,
	// empty string otherwise.
	GetOnUpdateExpr() string

	// IsComputed returns true iff the column is a computed column.
	IsComputed() bool

	// GetComputeExpr returns the column computed expression if it exists,
	// empty string otherwise.
	GetComputeExpr() string

	// IsHidden returns true iff the column is not visible.
	IsHidden() bool

	// IsInaccessible returns true iff the column is inaccessible.
	IsInaccessible() bool

	// IsExpressionIndexColumn returns true iff the column is an an inaccessible
	// virtual computed column that represents an expression in an expression
	// index.
	IsExpressionIndexColumn() bool

	// NumUsesSequences returns the number of sequences used by this column.
	NumUsesSequences() int

	// GetUsesSequenceID returns the ID of a sequence used by this column.
	GetUsesSequenceID(usesSequenceOrdinal int) descpb.ID

	// NumOwnsSequences returns the number of sequences owned by this column.
	NumOwnsSequences() int

	// GetOwnsSequenceID returns the ID of a sequence owned by this column.
	GetOwnsSequenceID(ownsSequenceOrdinal int) descpb.ID

	// IsVirtual returns true iff the column is a virtual column.
	IsVirtual() bool

	// CheckCanBeInboundFKRef returns whether the given column can be on the
	// referenced (target) side of a foreign key relation.
	CheckCanBeInboundFKRef() error

	// CheckCanBeOutboundFKRef returns whether the given column can be on the
	// referencing (origin) side of a foreign key relation.
	CheckCanBeOutboundFKRef() error

	// GetPGAttributeNum returns the PGAttributeNum of the column descriptor
	// if the PGAttributeNum is set (non-zero). Returns the ID of the
	// column descriptor if the PGAttributeNum is not set.
	GetPGAttributeNum() uint32

	// IsSystemColumn returns true iff the column is a system column.
	IsSystemColumn() bool

	// IsGeneratedAsIdentity returns true iff the column is created
	// with GENERATED {ALWAYS | BY DEFAULT} AS IDENTITY syntax.
	IsGeneratedAsIdentity() bool

	// IsGeneratedAlwaysAsIdentity returns true iff the column is created
	// with GENERATED ALWAYS AS IDENTITY syntax.
	IsGeneratedAlwaysAsIdentity() bool

	// IsGeneratedByDefaultAsIdentity returns true iff the column is created
	// with GENERATED BY DEFAULT AS IDENTITY syntax.
	IsGeneratedByDefaultAsIdentity() bool

	// GetGeneratedAsIdentityType returns the type of how the column was
	// created as an IDENTITY column.
	// If the column is created with `GENERATED ALWAYS AS IDENTITY` syntax,
	// it will return descpb.GeneratedAsIdentityType_GENERATED_ALWAYS;
	// if the column is created with `GENERATED BY DEFAULT AS IDENTITY` syntax,
	// it will return descpb.GeneratedAsIdentityType_GENERATED_BY_DEFAULT;
	// otherwise, returns descpb.GeneratedAsIdentityType_NOT_IDENTITY_COLUMN.
	GetGeneratedAsIdentityType() catpb.GeneratedAsIdentityType

	// HasGeneratedAsIdentitySequenceOption returns true if there is a
	// customized sequence option when this column is created as a
	// `GENERATED AS IDENTITY` column.
	HasGeneratedAsIdentitySequenceOption() bool

	// GetGeneratedAsIdentitySequenceOption returns the column's `GENERATED AS
	// IDENTITY` sequence option if it exists, empty string otherwise.
	GetGeneratedAsIdentitySequenceOption() string
}

// ConstraintToUpdate is an interface around a constraint mutation.
type ConstraintToUpdate interface {
	TableElementMaybeMutation

	// ConstraintToUpdateDesc returns the underlying protobuf descriptor.
	ConstraintToUpdateDesc() *descpb.ConstraintToUpdate

	// GetName returns the name of this constraint update mutation.
	GetName() string

	// IsCheck returns true iff this is an update for a check constraint.
	IsCheck() bool

	// IsForeignKey returns true iff this is an update for a fk constraint.
	IsForeignKey() bool

	// IsNotNull returns true iff this is an update for a not-null constraint.
	IsNotNull() bool

	// IsUniqueWithoutIndex returns true iff this is an update for a unique
	// without index constraint.
	IsUniqueWithoutIndex() bool

	// Check returns the underlying check constraint, if there is one.
	Check() descpb.TableDescriptor_CheckConstraint

	// ForeignKey returns the underlying fk constraint, if there is one.
	ForeignKey() descpb.ForeignKeyConstraint

	// NotNullColumnID returns the underlying not-null column ID, if there is one.
	NotNullColumnID() descpb.ColumnID

	// UniqueWithoutIndex returns the underlying unique without index constraint, if
	// there is one.
	UniqueWithoutIndex() descpb.UniqueWithoutIndexConstraint

	// GetConstraintID returns the ID for the constraint.
	GetConstraintID() descpb.ConstraintID
}

// PrimaryKeySwap is an interface around a primary key swap mutation.
type PrimaryKeySwap interface {
	TableElementMaybeMutation

	// PrimaryKeySwapDesc returns the underlying protobuf descriptor.
	PrimaryKeySwapDesc() *descpb.PrimaryKeySwap

	// NumOldIndexes returns the number of old active indexes to swap out.
	NumOldIndexes() int

	// ForEachOldIndexIDs iterates through each of the old index IDs.
	// iterutil.Done is supported.
	ForEachOldIndexIDs(fn func(id descpb.IndexID) error) error

	// NumNewIndexes returns the number of new active indexes to swap in.
	NumNewIndexes() int

	// ForEachNewIndexIDs iterates through each of the new index IDs.
	// iterutil.Done is supported.
	ForEachNewIndexIDs(fn func(id descpb.IndexID) error) error

	// HasLocalityConfig returns true iff the locality config is swapped also.
	HasLocalityConfig() bool

	// LocalityConfigSwap returns the locality config swap, if there is one.
	LocalityConfigSwap() descpb.PrimaryKeySwap_LocalityConfigSwap
}

// ComputedColumnSwap is an interface around a computed column swap mutation.
type ComputedColumnSwap interface {
	TableElementMaybeMutation

	// ComputedColumnSwapDesc returns the underlying protobuf descriptor.
	ComputedColumnSwapDesc() *descpb.ComputedColumnSwap
}

// MaterializedViewRefresh is an interface around a materialized view refresh
// mutation.
type MaterializedViewRefresh interface {
	TableElementMaybeMutation

	// MaterializedViewRefreshDesc returns the underlying protobuf descriptor.
	MaterializedViewRefreshDesc() *descpb.MaterializedViewRefresh

	// ShouldBackfill returns true iff the query should be backfilled into the
	// indexes.
	ShouldBackfill() bool

	// AsOf returns the timestamp at which the query should be run.
	AsOf() hlc.Timestamp

	// ForEachIndexID iterates through each of the index IDs.
	// iterutil.Done is supported.
	ForEachIndexID(func(id descpb.IndexID) error) error

	// TableWithNewIndexes returns a new TableDescriptor based on the old one
	// but with the refreshed indexes put in.
	TableWithNewIndexes(tbl TableDescriptor) TableDescriptor
}

// Partitioning is an interface around an index partitioning.
type Partitioning interface {

	// PartitioningDesc returns the underlying protobuf descriptor.
	PartitioningDesc() *descpb.PartitioningDescriptor

	// DeepCopy returns a deep copy of the receiver.
	DeepCopy() Partitioning

	// FindPartitionByName recursively searches the partitioning for a partition
	// whose name matches the input and returns it, or nil if no match is found.
	FindPartitionByName(name string) Partitioning

	// ForEachPartitionName applies fn on each of the partition names in this
	// partition and recursively in its subpartitions.
	// Supports iterutil.Done.
	ForEachPartitionName(fn func(name string) error) error

	// ForEachList applies fn on each list element of the wrapped partitioning.
	// Supports iterutil.Done.
	ForEachList(fn func(name string, values [][]byte, subPartitioning Partitioning) error) error

	// ForEachRange applies fn on each range element of the wrapped partitioning.
	// Supports iterutil.Done.
	ForEachRange(fn func(name string, from, to []byte) error) error

	// NumColumns is how large of a prefix of the columns in an index are used in
	// the function mapping column values to partitions. If this is a
	// subpartition, this is offset to start from the end of the parent
	// partition's columns. If NumColumns is 0, then there is no partitioning.
	NumColumns() int

	// NumImplicitColumns specifies the number of columns that implicitly prefix a
	// given index. This occurs if a user specifies a PARTITION BY which is not a
	// prefix of the given index, in which case the ColumnIDs are added in front
	// of the index and this field denotes the number of columns added as a
	// prefix.
	// If NumImplicitColumns is 0, no implicit columns are defined for the index.
	NumImplicitColumns() int

	// NumLists returns the number of list elements in the underlying partitioning
	// descriptor.
	NumLists() int

	// NumRanges returns the number of range elements in the underlying
	// partitioning descriptor.
	NumRanges() int
}

func isIndexInSearchSet(desc TableDescriptor, opts IndexOpts, idx Index) bool {
	if !opts.NonPhysicalPrimaryIndex && idx.Primary() && !desc.IsPhysicalTable() {
		return false
	}
	if !opts.AddMutations && idx.Adding() {
		return false
	}
	if !opts.DropMutations && idx.Dropped() {
		return false
	}
	return true
}

// ForEachIndex runs f over each index in the table descriptor according to
// filter parameters in opts. Indexes are visited in their canonical order,
// see Index.Ordinal(). ForEachIndex supports iterutil.StopIteration().
func ForEachIndex(desc TableDescriptor, opts IndexOpts, f func(idx Index) error) error {
	for _, idx := range desc.AllIndexes() {
		if !isIndexInSearchSet(desc, opts, idx) {
			continue
		}
		if err := f(idx); err != nil {
			if iterutil.Done(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

func forEachIndex(slice []Index, f func(idx Index) error) error {
	for _, idx := range slice {
		if err := f(idx); err != nil {
			if iterutil.Done(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

// ForEachActiveIndex is like ForEachIndex over ActiveIndexes().
func ForEachActiveIndex(desc TableDescriptor, f func(idx Index) error) error {
	return forEachIndex(desc.ActiveIndexes(), f)
}

// ForEachNonDropIndex is like ForEachIndex over NonDropIndexes().
func ForEachNonDropIndex(desc TableDescriptor, f func(idx Index) error) error {
	return forEachIndex(desc.NonDropIndexes(), f)
}

// ForEachPartialIndex is like ForEachIndex over PartialIndexes().
func ForEachPartialIndex(desc TableDescriptor, f func(idx Index) error) error {
	return forEachIndex(desc.PartialIndexes(), f)
}

// ForEachNonPrimaryIndex is like ForEachIndex over
// NonPrimaryIndexes().
func ForEachNonPrimaryIndex(desc TableDescriptor, f func(idx Index) error) error {
	return forEachIndex(desc.NonPrimaryIndexes(), f)
}

// ForEachPublicNonPrimaryIndex is like ForEachIndex over
// PublicNonPrimaryIndexes().
func ForEachPublicNonPrimaryIndex(desc TableDescriptor, f func(idx Index) error) error {
	return forEachIndex(desc.PublicNonPrimaryIndexes(), f)
}

// ForEachWritableNonPrimaryIndex is like ForEachIndex over
// WritableNonPrimaryIndexes().
func ForEachWritableNonPrimaryIndex(desc TableDescriptor, f func(idx Index) error) error {
	return forEachIndex(desc.WritableNonPrimaryIndexes(), f)
}

// ForEachDeletableNonPrimaryIndex is like ForEachIndex over
// DeletableNonPrimaryIndexes().
func ForEachDeletableNonPrimaryIndex(desc TableDescriptor, f func(idx Index) error) error {
	return forEachIndex(desc.DeletableNonPrimaryIndexes(), f)
}

// ForEachDeleteOnlyNonPrimaryIndex is like ForEachIndex over
// DeleteOnlyNonPrimaryIndexes().
func ForEachDeleteOnlyNonPrimaryIndex(desc TableDescriptor, f func(idx Index) error) error {
	return forEachIndex(desc.DeleteOnlyNonPrimaryIndexes(), f)
}

// FindIndex returns the first index for which test returns true, nil otherwise,
// according to the parameters in opts just like ForEachIndex.
// Indexes are visited in their canonical order, see Index.Ordinal().
func FindIndex(desc TableDescriptor, opts IndexOpts, test func(idx Index) bool) Index {
	for _, idx := range desc.AllIndexes() {
		if !isIndexInSearchSet(desc, opts, idx) {
			continue
		}
		if test(idx) {
			return idx
		}
	}
	return nil
}

func findIndex(slice []Index, test func(idx Index) bool) Index {
	for _, idx := range slice {
		if test(idx) {
			return idx
		}
	}
	return nil
}

// FindActiveIndex returns the first index in ActiveIndex() for which test
// returns true.
func FindActiveIndex(desc TableDescriptor, test func(idx Index) bool) Index {
	return findIndex(desc.ActiveIndexes(), test)
}

// FindNonDropIndex returns the first index in NonDropIndex() for which test
// returns true.
func FindNonDropIndex(desc TableDescriptor, test func(idx Index) bool) Index {
	return findIndex(desc.NonDropIndexes(), test)
}

// FindPartialIndex returns the first index in PartialIndex() for which test
// returns true.
func FindPartialIndex(desc TableDescriptor, test func(idx Index) bool) Index {
	return findIndex(desc.PartialIndexes(), test)
}

// FindPublicNonPrimaryIndex returns the first index in PublicNonPrimaryIndex()
// for which test returns true.
func FindPublicNonPrimaryIndex(desc TableDescriptor, test func(idx Index) bool) Index {
	return findIndex(desc.PublicNonPrimaryIndexes(), test)
}

// FindWritableNonPrimaryIndex returns the first index in
// WritableNonPrimaryIndex() for which test returns true.
func FindWritableNonPrimaryIndex(desc TableDescriptor, test func(idx Index) bool) Index {
	return findIndex(desc.WritableNonPrimaryIndexes(), test)
}

// FindDeletableNonPrimaryIndex returns the first index in
// DeletableNonPrimaryIndex() for which test returns true.
func FindDeletableNonPrimaryIndex(desc TableDescriptor, test func(idx Index) bool) Index {
	return findIndex(desc.DeletableNonPrimaryIndexes(), test)
}

// FindNonPrimaryIndex returns the first index in
// NonPrimaryIndex() for which test returns true.
func FindNonPrimaryIndex(desc TableDescriptor, test func(idx Index) bool) Index {
	return findIndex(desc.NonPrimaryIndexes(), test)
}

// FindDeleteOnlyNonPrimaryIndex returns the first index in
// DeleteOnlyNonPrimaryIndex() for which test returns true.
func FindDeleteOnlyNonPrimaryIndex(desc TableDescriptor, test func(idx Index) bool) Index {
	return findIndex(desc.DeleteOnlyNonPrimaryIndexes(), test)
}

// UserDefinedTypeColsHaveSameVersion returns whether one table descriptor's
// columns with user defined type metadata have the same versions of metadata
// as in the other descriptor. Note that this function is only valid on two
// descriptors representing the same table at the same version.
func UserDefinedTypeColsHaveSameVersion(desc TableDescriptor, otherDesc TableDescriptor) bool {
	otherCols := otherDesc.UserDefinedTypeColumns()
	for i, thisCol := range desc.UserDefinedTypeColumns() {
		this, other := thisCol.GetType(), otherCols[i].GetType()
		if this.TypeMeta.Version != other.TypeMeta.Version {
			return false
		}
	}
	return true
}

// ColumnIDToOrdinalMap returns a map from Column ID to the ordinal
// position of that column.
func ColumnIDToOrdinalMap(columns []Column) TableColMap {
	var m TableColMap
	for _, col := range columns {
		m.Set(col.GetID(), col.Ordinal())
	}
	return m
}

// ColumnTypes returns the types of the given columns
func ColumnTypes(columns []Column) []*types.T {
	return ColumnTypesWithInvertedCol(columns, nil /* invertedCol */)
}

// ColumnTypesWithInvertedCol returns the types of all given columns,
// If invertedCol is non-nil, substitutes the type of the inverted
// column instead of the column with the same ID.
func ColumnTypesWithInvertedCol(columns []Column, invertedCol Column) []*types.T {
	t := make([]*types.T, len(columns))
	for i, col := range columns {
		t[i] = col.GetType()
		if invertedCol != nil && col.GetID() == invertedCol.GetID() {
			t[i] = invertedCol.GetType()
		}
	}
	return t
}

// ColumnNeedsBackfill returns true if adding or dropping (according to
// the direction) the given column requires backfill.
func ColumnNeedsBackfill(col Column) bool {
	if col.IsVirtual() {
		// Virtual columns are not stored in the primary index, so they do not need
		// backfill.
		return false
	}
	if col.Dropped() {
		// In all other cases, DROP requires backfill.
		return true
	}
	// ADD requires backfill for:
	//  - columns with non-NULL default value
	//  - computed columns
	//  - non-nullable columns (note: if a non-nullable column doesn't have a
	//    default value, the backfill will fail unless the table is empty).
	if col.ColumnDesc().HasNullDefault() {
		return false
	}
	return col.HasDefault() || !col.IsNullable() || col.IsComputed()
}

// HasConcurrentSchemaChanges returns whether the table descriptor is undergoing
// concurrent schema changes.
func HasConcurrentSchemaChanges(table TableDescriptor) bool {
	// TODO(ajwerner): For now we simply check for the absence of mutations. Once
	// we start implementing schema changes with ops to be executed during
	// statement execution, we'll have to take into account mutations that were
	// written in this transaction.
	return len(table.AllMutations()) > 0
}
