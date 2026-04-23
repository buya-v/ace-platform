package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/garudax-platform/warehouse-service/internal/types"
)

// PostgresStore implements DataStore backed by PostgreSQL.
// It uses the warehouse schema defined in V9__warehouse_tables.sql.
type PostgresStore struct {
	db *sql.DB
}

// Compile-time interface check.
var _ DataStore = (*PostgresStore)(nil)

// NewPostgresStore creates a new PostgreSQL-backed store.
// The caller must register the pgx driver before calling this:
//
//	import _ "github.com/jackc/pgx/v5/stdlib"
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// DB returns the underlying *sql.DB for health checks or lifecycle management.
func (p *PostgresStore) DB() *sql.DB {
	return p.db
}

// --- helpers ---

func scanDecimal(s string) types.Decimal {
	d, _ := types.ParseDecimal(s)
	return d
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func fromNullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func fromNullTime(nt sql.NullTime) *time.Time {
	if nt.Valid {
		t := nt.Time
		return &t
	}
	return nil
}

// --- Facility operations ---

func (p *PostgresStore) CreateFacility(f *types.Facility) error {
	now := time.Now().UTC()
	f.CreatedAt = now
	f.UpdatedAt = now

	var id string
	err := p.db.QueryRow(`
		INSERT INTO ace_warehouse.facilities
			(facility_code, name, operator_id, license_number, license_expiry,
			 address, latitude, longitude, region, total_capacity, capacity_unit, status,
			 created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING facility_id`,
		f.FacilityCode, f.Name, f.OperatorID, f.LicenseNumber, nullStr(f.LicenseExpiry),
		f.Address, nullStr(f.Latitude), nullStr(f.Longitude),
		f.Region, f.TotalCapacity.String(), f.CapacityUnit, string(f.Status),
		f.CreatedAt, f.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("insert facility: %w", err)
	}
	f.FacilityID = id

	// Insert approved commodity associations
	for _, cid := range f.ApprovedCommodityIDs {
		_, err := p.db.Exec(`
			INSERT INTO ace_warehouse.facility_commodities (facility_id, commodity_id)
			VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, cid)
		if err != nil {
			return fmt.Errorf("insert facility commodity: %w", err)
		}
	}
	return nil
}

func (p *PostgresStore) GetFacility(facilityID string) (*types.Facility, error) {
	f := &types.Facility{}
	var capStr, latStr, lonStr, expiry sql.NullString
	var status string
	err := p.db.QueryRow(`
		SELECT facility_id, facility_code, name, operator_id, license_number,
		       license_expiry, address, latitude, longitude, region,
		       total_capacity, capacity_unit, status, created_at, updated_at
		FROM ace_warehouse.facilities WHERE facility_id = $1`, facilityID,
	).Scan(
		&f.FacilityID, &f.FacilityCode, &f.Name, &f.OperatorID, &f.LicenseNumber,
		&expiry, &f.Address, &latStr, &lonStr, &f.Region,
		&capStr, &f.CapacityUnit, &status, &f.CreatedAt, &f.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("facility %s not found", facilityID)
	}
	if err != nil {
		return nil, fmt.Errorf("get facility: %w", err)
	}
	f.TotalCapacity = scanDecimal(capStr.String)
	f.Latitude = fromNullStr(latStr)
	f.Longitude = fromNullStr(lonStr)
	f.LicenseExpiry = fromNullStr(expiry)
	f.Status = types.FacilityStatus(status)

	// Load approved commodities
	rows, err := p.db.Query(
		`SELECT commodity_id FROM ace_warehouse.facility_commodities WHERE facility_id = $1`, facilityID)
	if err != nil {
		return nil, fmt.Errorf("get facility commodities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		f.ApprovedCommodityIDs = append(f.ApprovedCommodityIDs, cid)
	}
	return f, rows.Err()
}

func (p *PostgresStore) UpdateFacility(f *types.Facility) error {
	f.UpdatedAt = time.Now().UTC()
	res, err := p.db.Exec(`
		UPDATE ace_warehouse.facilities SET
			name=$2, license_number=$3, license_expiry=$4, address=$5,
			latitude=$6, longitude=$7, region=$8, total_capacity=$9,
			capacity_unit=$10, status=$11, updated_at=$12
		WHERE facility_id = $1`,
		f.FacilityID, f.Name, f.LicenseNumber, nullStr(f.LicenseExpiry),
		f.Address, nullStr(f.Latitude), nullStr(f.Longitude),
		f.Region, f.TotalCapacity.String(), f.CapacityUnit, string(f.Status),
		f.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update facility: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("facility %s not found", f.FacilityID)
	}

	// Replace commodity associations
	_, _ = p.db.Exec(`DELETE FROM ace_warehouse.facility_commodities WHERE facility_id = $1`, f.FacilityID)
	for _, cid := range f.ApprovedCommodityIDs {
		_, _ = p.db.Exec(`INSERT INTO ace_warehouse.facility_commodities (facility_id, commodity_id)
			VALUES ($1, $2) ON CONFLICT DO NOTHING`, f.FacilityID, cid)
	}
	return nil
}

func (p *PostgresStore) ListFacilities(region string, status types.FacilityStatus) []*types.Facility {
	query := `SELECT facility_id, facility_code, name, operator_id, license_number,
		       license_expiry, address, latitude, longitude, region,
		       total_capacity, capacity_unit, status, created_at, updated_at
		FROM ace_warehouse.facilities WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if region != "" {
		query += fmt.Sprintf(" AND region = $%d", idx)
		args = append(args, region)
		idx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, string(status))
		idx++
	}
	query += " ORDER BY created_at"

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []*types.Facility
	for rows.Next() {
		f := &types.Facility{}
		var capStr, latStr, lonStr, expiry sql.NullString
		var st string
		if err := rows.Scan(
			&f.FacilityID, &f.FacilityCode, &f.Name, &f.OperatorID, &f.LicenseNumber,
			&expiry, &f.Address, &latStr, &lonStr, &f.Region,
			&capStr, &f.CapacityUnit, &st, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			continue
		}
		f.TotalCapacity = scanDecimal(capStr.String)
		f.Latitude = fromNullStr(latStr)
		f.Longitude = fromNullStr(lonStr)
		f.LicenseExpiry = fromNullStr(expiry)
		f.Status = types.FacilityStatus(st)
		result = append(result, f)
	}
	return result
}

// --- Inspection operations ---

func (p *PostgresStore) CreateInspection(insp *types.Inspection) error {
	// Verify facility exists
	var exists bool
	err := p.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM ace_warehouse.facilities WHERE facility_id=$1)`,
		insp.FacilityID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("facility %s not found", insp.FacilityID)
	}

	now := time.Now().UTC()
	insp.CreatedAt = now
	insp.UpdatedAt = now

	var id string
	err = p.db.QueryRow(`
		INSERT INTO ace_warehouse.inspections
			(facility_id, commodity_id, lot_number, inspector_id, inspection_type,
			 status, scheduled_date, completed_date,
			 gross_weight, net_weight, moisture_pct, foreign_matter_pct,
			 protein_pct, test_weight, grade_assigned, defects, notes,
			 certificate_number, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING inspection_id`,
		insp.FacilityID, insp.CommodityID, insp.LotNumber, insp.InspectorID,
		string(insp.InspectionType), string(insp.Status),
		insp.ScheduledDate, nullStr(insp.CompletedDate),
		nullDecStr(insp.GrossWeight), nullDecStr(insp.NetWeight),
		nullDecStr(insp.MoisturePct), nullDecStr(insp.ForeignMatterPct),
		nullDecStr(insp.ProteinPct), nullDecStr(insp.TestWeight),
		nullStr(insp.GradeAssigned), nullJsonStr(insp.Defects),
		nullStr(insp.Notes), nullStr(insp.CertificateNumber),
		insp.CreatedAt, insp.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("insert inspection: %w", err)
	}
	insp.InspectionID = id
	return nil
}

func nullDecStr(d types.Decimal) sql.NullString {
	if d.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: d.String(), Valid: true}
}

func nullJsonStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	// Validate it's valid JSON, otherwise wrap as string
	if json.Valid([]byte(s)) {
		return sql.NullString{String: s, Valid: true}
	}
	b, _ := json.Marshal(s)
	return sql.NullString{String: string(b), Valid: true}
}

func (p *PostgresStore) GetInspection(inspectionID string) (*types.Inspection, error) {
	insp := &types.Inspection{}
	var status, inspType string
	var completedDate, gradeAssigned, notes, certNum sql.NullString
	var defects sql.NullString
	var grossW, netW, moistPct, fmPct, protPct, testW sql.NullString

	err := p.db.QueryRow(`
		SELECT inspection_id, facility_id, commodity_id, lot_number, inspector_id,
		       inspection_type, status, scheduled_date, completed_date,
		       gross_weight, net_weight, moisture_pct, foreign_matter_pct,
		       protein_pct, test_weight, grade_assigned, defects, notes,
		       certificate_number, created_at, updated_at
		FROM ace_warehouse.inspections WHERE inspection_id = $1`, inspectionID,
	).Scan(
		&insp.InspectionID, &insp.FacilityID, &insp.CommodityID, &insp.LotNumber,
		&insp.InspectorID, &inspType, &status, &insp.ScheduledDate,
		&completedDate, &grossW, &netW, &moistPct, &fmPct, &protPct, &testW,
		&gradeAssigned, &defects, &notes, &certNum,
		&insp.CreatedAt, &insp.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("inspection %s not found", inspectionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get inspection: %w", err)
	}

	insp.InspectionType = types.InspectionType(inspType)
	insp.Status = types.InspectionStatus(status)
	insp.CompletedDate = fromNullStr(completedDate)
	insp.GrossWeight = scanDecimal(fromNullStr(grossW))
	insp.NetWeight = scanDecimal(fromNullStr(netW))
	insp.MoisturePct = scanDecimal(fromNullStr(moistPct))
	insp.ForeignMatterPct = scanDecimal(fromNullStr(fmPct))
	insp.ProteinPct = scanDecimal(fromNullStr(protPct))
	insp.TestWeight = scanDecimal(fromNullStr(testW))
	insp.GradeAssigned = fromNullStr(gradeAssigned)
	insp.Defects = fromNullStr(defects)
	insp.Notes = fromNullStr(notes)
	insp.CertificateNumber = fromNullStr(certNum)
	return insp, nil
}

func (p *PostgresStore) UpdateInspection(insp *types.Inspection) error {
	insp.UpdatedAt = time.Now().UTC()
	res, err := p.db.Exec(`
		UPDATE ace_warehouse.inspections SET
			status=$2, completed_date=$3,
			gross_weight=$4, net_weight=$5, moisture_pct=$6, foreign_matter_pct=$7,
			protein_pct=$8, test_weight=$9, grade_assigned=$10, defects=$11,
			notes=$12, certificate_number=$13, updated_at=$14
		WHERE inspection_id = $1`,
		insp.InspectionID, string(insp.Status), nullStr(insp.CompletedDate),
		nullDecStr(insp.GrossWeight), nullDecStr(insp.NetWeight),
		nullDecStr(insp.MoisturePct), nullDecStr(insp.ForeignMatterPct),
		nullDecStr(insp.ProteinPct), nullDecStr(insp.TestWeight),
		nullStr(insp.GradeAssigned), nullJsonStr(insp.Defects),
		nullStr(insp.Notes), nullStr(insp.CertificateNumber),
		insp.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update inspection: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("inspection %s not found", insp.InspectionID)
	}
	return nil
}

// --- Receipt operations ---

func (p *PostgresStore) CreateReceipt(r *types.Receipt) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check lot uniqueness among active receipts
	var existingID string
	err = tx.QueryRow(`
		SELECT receipt_id FROM ace_warehouse.receipts
		WHERE facility_id=$1 AND commodity_id=$2 AND lot_number=$3
		  AND status IN ('ACTIVE','PLEDGED','DELIVERY_PENDING')
		LIMIT 1`, r.FacilityID, r.CommodityID, r.LotNumber,
	).Scan(&existingID)
	if err == nil {
		return fmt.Errorf("active receipt %s already exists for lot %s at facility %s",
			existingID, r.LotNumber, r.FacilityID)
	}
	if err != sql.ErrNoRows {
		return err
	}

	// Check facility exists and is active
	var facStatus string
	var totalCapStr string
	err = tx.QueryRow(`
		SELECT status, total_capacity FROM ace_warehouse.facilities WHERE facility_id=$1`,
		r.FacilityID).Scan(&facStatus, &totalCapStr)
	if err == sql.ErrNoRows {
		return fmt.Errorf("facility %s not found", r.FacilityID)
	}
	if err != nil {
		return err
	}
	if facStatus != string(types.FacilityStatusActive) {
		return fmt.Errorf("facility %s is not active (status: %s)", r.FacilityID, facStatus)
	}

	// Check capacity
	totalCap := scanDecimal(totalCapStr)
	var usedCapStr sql.NullString
	err = tx.QueryRow(`
		SELECT COALESCE(SUM(quantity), '0') FROM ace_warehouse.receipts
		WHERE facility_id=$1 AND status IN ('ACTIVE','PLEDGED','DELIVERY_PENDING')`,
		r.FacilityID).Scan(&usedCapStr)
	if err != nil {
		return err
	}
	usedCap := scanDecimal(fromNullStr(usedCapStr))
	availCap := totalCap.Sub(usedCap)
	if r.Quantity.GreaterThan(availCap) {
		return fmt.Errorf("insufficient facility capacity: available=%s, requested=%s",
			availCap.String(), r.Quantity.String())
	}

	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	r.IssuedAt = now
	if r.ExpiresAt.IsZero() {
		r.ExpiresAt = now.AddDate(1, 0, 0)
	}
	r.Status = types.ReceiptStatusActive

	var id string
	err = tx.QueryRow(`
		INSERT INTO ace_warehouse.receipts
			(facility_id, holder_id, commodity_id, grade, quantity, gross_quantity,
			 unit, lot_number, storage_location, harvest_year, inspection_id,
			 status, pledged_to, issued_at, expires_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING receipt_id, receipt_number`,
		r.FacilityID, r.HolderID, r.CommodityID, r.Grade,
		r.Quantity.String(), r.GrossQuantity.String(),
		r.Unit, r.LotNumber, nullStr(r.StorageLocation), r.HarvestYear,
		nullStr(r.InspectionID), string(r.Status), nullStr(r.PledgedTo),
		r.IssuedAt, r.ExpiresAt, r.CreatedAt, r.UpdatedAt,
	).Scan(&id, &r.ReceiptNumber)
	if err != nil {
		return fmt.Errorf("insert receipt: %w", err)
	}
	r.ReceiptID = id

	// Record ISSUED receipt event
	_, err = tx.Exec(`
		INSERT INTO ace_warehouse.receipt_events
			(receipt_id, event_type, to_holder_id, created_at)
		VALUES ($1, 'ISSUED', $2, $3)`, id, r.HolderID, now)
	if err != nil {
		return fmt.Errorf("insert receipt event: %w", err)
	}

	// Record inventory deposit
	_, err = tx.Exec(`
		INSERT INTO ace_warehouse.inventory_events
			(facility_id, commodity_id, lot_number, event_type, quantity,
			 reference_id, reference_type, created_by, created_at)
		VALUES ($1,$2,$3,'DEPOSIT',$4,$5,'RECEIPT',$6,$7)`,
		r.FacilityID, r.CommodityID, r.LotNumber, r.Quantity.String(),
		id, r.HolderID, now)
	if err != nil {
		return fmt.Errorf("insert inventory event: %w", err)
	}

	return tx.Commit()
}

func (p *PostgresStore) scanReceipt(scanner interface {
	Scan(dest ...interface{}) error
}) (*types.Receipt, error) {
	r := &types.Receipt{}
	var status string
	var qtyStr, grossQtyStr string
	var storLoc, inspID, pledgedTo sql.NullString
	var cancelledAt, deliveredAt sql.NullTime

	err := scanner.Scan(
		&r.ReceiptID, &r.ReceiptNumber, &r.FacilityID, &r.HolderID, &r.CommodityID,
		&r.Grade, &qtyStr, &grossQtyStr, &r.Unit, &r.LotNumber,
		&storLoc, &r.HarvestYear, &inspID, &status, &pledgedTo,
		&r.IssuedAt, &r.ExpiresAt, &cancelledAt, &deliveredAt,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	r.Quantity = scanDecimal(qtyStr)
	r.GrossQuantity = scanDecimal(grossQtyStr)
	r.StorageLocation = fromNullStr(storLoc)
	r.InspectionID = fromNullStr(inspID)
	r.Status = types.ReceiptStatus(status)
	r.PledgedTo = fromNullStr(pledgedTo)
	return r, nil
}

const receiptCols = `receipt_id, receipt_number, facility_id, holder_id, commodity_id,
	grade, quantity, gross_quantity, unit, lot_number,
	storage_location, harvest_year, inspection_id, status, pledged_to,
	issued_at, expires_at, cancelled_at, delivered_at, created_at, updated_at`

func (p *PostgresStore) GetReceipt(receiptID string) (*types.Receipt, error) {
	row := p.db.QueryRow(
		`SELECT `+receiptCols+` FROM ace_warehouse.receipts WHERE receipt_id = $1`, receiptID)
	r, err := p.scanReceipt(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	if err != nil {
		return nil, fmt.Errorf("get receipt: %w", err)
	}
	return r, nil
}

func (p *PostgresStore) ListReceipts(holderID, facilityID, commodityID string, status types.ReceiptStatus) []*types.Receipt {
	query := `SELECT ` + receiptCols + ` FROM ace_warehouse.receipts WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if holderID != "" {
		query += fmt.Sprintf(" AND holder_id = $%d", idx)
		args = append(args, holderID)
		idx++
	}
	if facilityID != "" {
		query += fmt.Sprintf(" AND facility_id = $%d", idx)
		args = append(args, facilityID)
		idx++
	}
	if commodityID != "" {
		query += fmt.Sprintf(" AND commodity_id = $%d", idx)
		args = append(args, commodityID)
		idx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, string(status))
		idx++
	}
	query += " ORDER BY created_at"

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []*types.Receipt
	for rows.Next() {
		r, err := p.scanReceipt(rows)
		if err != nil {
			continue
		}
		result = append(result, r)
	}
	return result
}

func (p *PostgresStore) TransferReceipt(receiptID, newHolderID string) (*types.Receipt, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Load current receipt
	var oldHolder, status string
	err = tx.QueryRow(
		`SELECT holder_id, status FROM ace_warehouse.receipts WHERE receipt_id=$1 FOR UPDATE`,
		receiptID).Scan(&oldHolder, &status)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	if err != nil {
		return nil, err
	}
	if status != string(types.ReceiptStatusActive) {
		return nil, fmt.Errorf("receipt %s cannot be transferred (status: %s)", receiptID, status)
	}

	now := time.Now().UTC()
	_, err = tx.Exec(`
		UPDATE ace_warehouse.receipts SET holder_id=$2, updated_at=$3 WHERE receipt_id=$1`,
		receiptID, newHolderID, now)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`
		INSERT INTO ace_warehouse.receipt_events
			(receipt_id, event_type, from_holder_id, to_holder_id, created_at)
		VALUES ($1, 'TRANSFERRED', $2, $3, $4)`,
		receiptID, oldHolder, newHolderID, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.GetReceipt(receiptID)
}

func (p *PostgresStore) CancelReceipt(receiptID, reason string) (*types.Receipt, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status, facilityID, commodityID, lotNumber, holderID string
	var qtyStr string
	err = tx.QueryRow(`
		SELECT status, facility_id, commodity_id, lot_number, holder_id, quantity
		FROM ace_warehouse.receipts WHERE receipt_id=$1 FOR UPDATE`,
		receiptID).Scan(&status, &facilityID, &commodityID, &lotNumber, &holderID, &qtyStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	if err != nil {
		return nil, err
	}
	if status != string(types.ReceiptStatusActive) {
		return nil, fmt.Errorf("receipt %s cannot be cancelled (status: %s)", receiptID, status)
	}

	now := time.Now().UTC()
	_, err = tx.Exec(`
		UPDATE ace_warehouse.receipts SET status='CANCELLED', cancelled_at=$2, updated_at=$2
		WHERE receipt_id=$1`, receiptID, now)
	if err != nil {
		return nil, err
	}

	// Audit event
	var metaJSON sql.NullString
	if reason != "" {
		b, _ := json.Marshal(map[string]string{"reason": reason})
		metaJSON = sql.NullString{String: string(b), Valid: true}
	}
	_, err = tx.Exec(`
		INSERT INTO ace_warehouse.receipt_events
			(receipt_id, event_type, metadata, created_at)
		VALUES ($1, 'CANCELLED', $2, $3)`,
		receiptID, metaJSON, now)
	if err != nil {
		return nil, err
	}

	// Inventory withdrawal (negative quantity)
	qty := scanDecimal(qtyStr)
	negQty := types.DecimalFromRaw(-qty.Raw())
	_, err = tx.Exec(`
		INSERT INTO ace_warehouse.inventory_events
			(facility_id, commodity_id, lot_number, event_type, quantity,
			 reference_id, reference_type, created_by, created_at)
		VALUES ($1,$2,$3,'WITHDRAWAL',$4,$5,'RECEIPT_CANCEL',$6,$7)`,
		facilityID, commodityID, lotNumber, negQty.String(),
		receiptID, holderID, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.GetReceipt(receiptID)
}

func (p *PostgresStore) PledgeReceipt(receiptID, clearingMemberID string) (*types.Receipt, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRow(`SELECT status FROM ace_warehouse.receipts WHERE receipt_id=$1 FOR UPDATE`,
		receiptID).Scan(&status)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	if err != nil {
		return nil, err
	}
	if status != string(types.ReceiptStatusActive) {
		return nil, fmt.Errorf("receipt %s cannot be pledged (status: %s)", receiptID, status)
	}

	now := time.Now().UTC()
	_, err = tx.Exec(`
		UPDATE ace_warehouse.receipts SET status='PLEDGED', pledged_to=$2, updated_at=$3
		WHERE receipt_id=$1`, receiptID, clearingMemberID, now)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`
		INSERT INTO ace_warehouse.receipt_events
			(receipt_id, event_type, pledged_to, created_at)
		VALUES ($1, 'PLEDGED', $2, $3)`,
		receiptID, clearingMemberID, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.GetReceipt(receiptID)
}

func (p *PostgresStore) ReleaseReceipt(receiptID, clearingMemberID string) (*types.Receipt, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status, pledgedTo string
	err = tx.QueryRow(`SELECT status, COALESCE(pledged_to::text,'') FROM ace_warehouse.receipts WHERE receipt_id=$1 FOR UPDATE`,
		receiptID).Scan(&status, &pledgedTo)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("receipt %s not found", receiptID)
	}
	if err != nil {
		return nil, err
	}
	if status != string(types.ReceiptStatusPledged) {
		return nil, fmt.Errorf("receipt %s is not pledged (status: %s)", receiptID, status)
	}
	if pledgedTo != clearingMemberID {
		return nil, fmt.Errorf("receipt %s is pledged to %s, not %s", receiptID, pledgedTo, clearingMemberID)
	}

	now := time.Now().UTC()
	_, err = tx.Exec(`
		UPDATE ace_warehouse.receipts SET status='ACTIVE', pledged_to=NULL, updated_at=$2
		WHERE receipt_id=$1`, receiptID, now)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`
		INSERT INTO ace_warehouse.receipt_events
			(receipt_id, event_type, pledged_to, created_at)
		VALUES ($1, 'RELEASED', $2, $3)`,
		receiptID, clearingMemberID, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.GetReceipt(receiptID)
}

// --- Delivery operations ---

func (p *PostgresStore) CreateDelivery(d *types.DeliveryInstruction) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check receipt
	var rStatus, rHolderID, rFacilityID string
	var rQtyStr string
	err = tx.QueryRow(`
		SELECT status, holder_id, facility_id, quantity
		FROM ace_warehouse.receipts WHERE receipt_id=$1 FOR UPDATE`,
		d.ReceiptID).Scan(&rStatus, &rHolderID, &rFacilityID, &rQtyStr)
	if err == sql.ErrNoRows {
		return fmt.Errorf("receipt %s not found", d.ReceiptID)
	}
	if err != nil {
		return err
	}
	if rStatus != string(types.ReceiptStatusActive) && rStatus != string(types.ReceiptStatusPledged) {
		return fmt.Errorf("receipt %s cannot be delivered (status: %s)", d.ReceiptID, rStatus)
	}

	now := time.Now().UTC()

	// Mark receipt as delivery pending
	_, err = tx.Exec(`
		UPDATE ace_warehouse.receipts SET status='DELIVERY_PENDING', updated_at=$2
		WHERE receipt_id=$1`, d.ReceiptID, now)
	if err != nil {
		return err
	}

	d.SellerID = rHolderID
	d.FacilityID = rFacilityID
	d.Quantity = scanDecimal(rQtyStr)
	d.Status = types.DeliveryStatusPending
	d.CreatedAt = now
	d.UpdatedAt = now

	dt := d.DeliveryType
	if dt == "" {
		dt = types.DeliveryTypePhysical
	}

	var id string
	err = tx.QueryRow(`
		INSERT INTO ace_warehouse.deliveries
			(receipt_id, obligation_id, seller_id, buyer_id, delivery_type,
			 facility_id, destination_id, quantity, scheduled_date, status,
			 created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING delivery_id`,
		d.ReceiptID, d.ObligationID, d.SellerID, d.BuyerID, string(dt),
		d.FacilityID, nullStr(d.DestinationID), d.Quantity.String(),
		nullStr(d.ScheduledDate), string(d.Status),
		d.CreatedAt, d.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("insert delivery: %w", err)
	}
	d.DeliveryID = id

	// Receipt event
	_, err = tx.Exec(`
		INSERT INTO ace_warehouse.receipt_events
			(receipt_id, event_type, created_at)
		VALUES ($1, 'DELIVERY_INITIATED', $2)`, d.ReceiptID, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (p *PostgresStore) scanDelivery(scanner interface {
	Scan(dest ...interface{}) error
}) (*types.DeliveryInstruction, error) {
	d := &types.DeliveryInstruction{}
	var status, delivType string
	var qtyStr string
	var destID, failReason sql.NullString
	var completedAt sql.NullTime

	err := scanner.Scan(
		&d.DeliveryID, &d.ReceiptID, &d.ObligationID, &d.SellerID, &d.BuyerID,
		&delivType, &d.FacilityID, &destID, &qtyStr, &d.ScheduledDate,
		&status, &failReason, &completedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	d.Quantity = scanDecimal(qtyStr)
	d.Status = types.DeliveryStatus(status)
	d.DeliveryType = types.DeliveryType(delivType)
	d.DestinationID = fromNullStr(destID)
	d.FailureReason = fromNullStr(failReason)
	d.CompletedAt = fromNullTime(completedAt)
	return d, nil
}

const deliveryCols = `delivery_id, receipt_id, obligation_id, seller_id, buyer_id,
	delivery_type, facility_id, destination_id, quantity, scheduled_date,
	status, failure_reason, completed_at, created_at, updated_at`

func (p *PostgresStore) GetDelivery(deliveryID string) (*types.DeliveryInstruction, error) {
	row := p.db.QueryRow(
		`SELECT `+deliveryCols+` FROM ace_warehouse.deliveries WHERE delivery_id = $1`, deliveryID)
	d, err := p.scanDelivery(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("delivery %s not found", deliveryID)
	}
	if err != nil {
		return nil, fmt.Errorf("get delivery: %w", err)
	}
	return d, nil
}

func (p *PostgresStore) ListDeliveries(sellerID, buyerID string, status types.DeliveryStatus) []*types.DeliveryInstruction {
	query := `SELECT ` + deliveryCols + ` FROM ace_warehouse.deliveries WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if sellerID != "" {
		query += fmt.Sprintf(" AND seller_id = $%d", idx)
		args = append(args, sellerID)
		idx++
	}
	if buyerID != "" {
		query += fmt.Sprintf(" AND buyer_id = $%d", idx)
		args = append(args, buyerID)
		idx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, string(status))
		idx++
	}
	query += " ORDER BY created_at"

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []*types.DeliveryInstruction
	for rows.Next() {
		d, err := p.scanDelivery(rows)
		if err != nil {
			continue
		}
		result = append(result, d)
	}
	return result
}

func (p *PostgresStore) CompleteDelivery(deliveryID string, success bool, failureReason string) (*types.DeliveryInstruction, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var dStatus, receiptID string
	err = tx.QueryRow(`
		SELECT status, receipt_id FROM ace_warehouse.deliveries WHERE delivery_id=$1 FOR UPDATE`,
		deliveryID).Scan(&dStatus, &receiptID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("delivery %s not found", deliveryID)
	}
	if err != nil {
		return nil, err
	}
	if dStatus != string(types.DeliveryStatusPending) && dStatus != string(types.DeliveryStatusInProgress) {
		return nil, fmt.Errorf("delivery %s cannot be completed (status: %s)", deliveryID, dStatus)
	}

	// Get receipt details for events
	var rFacilityID, rCommodityID, rLotNumber, rQtyStr, buyerID string
	err = tx.QueryRow(`
		SELECT r.facility_id, r.commodity_id, r.lot_number, r.quantity, d.buyer_id
		FROM ace_warehouse.receipts r
		JOIN ace_warehouse.deliveries d ON d.receipt_id = r.receipt_id
		WHERE d.delivery_id=$1`, deliveryID,
	).Scan(&rFacilityID, &rCommodityID, &rLotNumber, &rQtyStr, &buyerID)
	if err != nil {
		return nil, fmt.Errorf("receipt %s not found for delivery", receiptID)
	}

	now := time.Now().UTC()

	if success {
		_, err = tx.Exec(`
			UPDATE ace_warehouse.deliveries SET status='COMPLETED', completed_at=$2, updated_at=$2
			WHERE delivery_id=$1`, deliveryID, now)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`
			UPDATE ace_warehouse.receipts SET status='DELIVERED', delivered_at=$2, updated_at=$2
			WHERE receipt_id=$1`, receiptID, now)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`
			INSERT INTO ace_warehouse.receipt_events
				(receipt_id, event_type, created_at)
			VALUES ($1, 'DELIVERED', $2)`, receiptID, now)
		if err != nil {
			return nil, err
		}

		// Inventory withdrawal
		qty := scanDecimal(rQtyStr)
		negQty := types.DecimalFromRaw(-qty.Raw())
		_, err = tx.Exec(`
			INSERT INTO ace_warehouse.inventory_events
				(facility_id, commodity_id, lot_number, event_type, quantity,
				 reference_id, reference_type, created_by, created_at)
			VALUES ($1,$2,$3,'WITHDRAWAL',$4,$5,'DELIVERY',$6,$7)`,
			rFacilityID, rCommodityID, rLotNumber, negQty.String(),
			deliveryID, buyerID, now)
		if err != nil {
			return nil, err
		}
	} else {
		_, err = tx.Exec(`
			UPDATE ace_warehouse.deliveries SET status='FAILED', failure_reason=$2, completed_at=$3, updated_at=$3
			WHERE delivery_id=$1`, deliveryID, failureReason, now)
		if err != nil {
			return nil, err
		}

		// Revert receipt to active
		_, err = tx.Exec(`
			UPDATE ace_warehouse.receipts SET status='ACTIVE', updated_at=$2
			WHERE receipt_id=$1`, receiptID, now)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.GetDelivery(deliveryID)
}

// --- Inventory operations ---

func (p *PostgresStore) GetInventory(facilityID, commodityID string) ([]types.InventoryItem, types.Decimal) {
	query := `SELECT facility_id, commodity_id, lot_number, current_quantity
		FROM ace_warehouse.current_inventory WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if facilityID != "" {
		query += fmt.Sprintf(" AND facility_id = $%d", idx)
		args = append(args, facilityID)
		idx++
	}
	if commodityID != "" {
		query += fmt.Sprintf(" AND commodity_id = $%d", idx)
		args = append(args, commodityID)
		idx++
	}

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil, types.DecimalZero()
	}
	defer rows.Close()

	var items []types.InventoryItem
	total := types.DecimalZero()
	for rows.Next() {
		var item types.InventoryItem
		var qtyStr string
		if err := rows.Scan(&item.FacilityID, &item.CommodityID, &item.LotNumber, &qtyStr); err != nil {
			continue
		}
		item.CurrentQuantity = scanDecimal(qtyStr)
		item.Unit = "MT"
		items = append(items, item)
		total = total.Add(item.CurrentQuantity)
	}
	return items, total
}

func (p *PostgresStore) GetFacilityCapacity(facilityID string) (*types.FacilityCapacity, error) {
	var totalCapStr, usedCapStr, availCapStr, unit string
	err := p.db.QueryRow(`
		SELECT total_capacity, used_capacity, available_capacity, capacity_unit
		FROM ace_warehouse.facility_utilization WHERE facility_id = $1`,
		facilityID).Scan(&totalCapStr, &usedCapStr, &availCapStr, &unit)
	if err == sql.ErrNoRows {
		// Facility might exist but have no utilization row if view returns nothing
		// Fall back to checking facility directly
		var facCapStr, facUnit string
		err2 := p.db.QueryRow(`
			SELECT total_capacity, capacity_unit FROM ace_warehouse.facilities WHERE facility_id=$1`,
			facilityID).Scan(&facCapStr, &facUnit)
		if err2 == sql.ErrNoRows {
			return nil, fmt.Errorf("facility %s not found", facilityID)
		}
		if err2 != nil {
			return nil, err2
		}
		cap := scanDecimal(facCapStr)
		return &types.FacilityCapacity{
			TotalCapacity:     cap,
			UsedCapacity:      types.DecimalZero(),
			AvailableCapacity: cap,
			CapacityUnit:      facUnit,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get facility capacity: %w", err)
	}
	return &types.FacilityCapacity{
		TotalCapacity:     scanDecimal(totalCapStr),
		UsedCapacity:      scanDecimal(usedCapStr),
		AvailableCapacity: scanDecimal(availCapStr),
		CapacityUnit:      unit,
	}, nil
}

// --- Receipt events ---

func (p *PostgresStore) GetReceiptEvents(receiptID string) []types.ReceiptEvent {
	rows, err := p.db.Query(`
		SELECT event_id, receipt_id, event_type,
		       COALESCE(from_holder_id::text, ''), COALESCE(to_holder_id::text, ''),
		       COALESCE(pledged_to::text, ''), COALESCE(metadata::text, ''),
		       created_at
		FROM ace_warehouse.receipt_events
		WHERE receipt_id = $1
		ORDER BY created_at`, receiptID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var events []types.ReceiptEvent
	for rows.Next() {
		var e types.ReceiptEvent
		var eventType string
		if err := rows.Scan(
			&e.EventID, &e.ReceiptID, &eventType,
			&e.FromHolderID, &e.ToHolderID, &e.PledgedTo, &e.Metadata,
			&e.CreatedAt,
		); err != nil {
			continue
		}
		e.EventType = types.ReceiptEventType(eventType)
		events = append(events, e)
	}
	return events
}
