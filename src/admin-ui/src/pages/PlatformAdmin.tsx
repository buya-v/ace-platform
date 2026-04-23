import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchTenants, createTenant, updateTenantStatus } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { DataGrid, Column } from '../components/DataGrid';
import { useToast } from '../contexts/ToastContext';
import styles from './PlatformAdmin.module.css';

// ─── Types ───────────────────────────────────────────────────────────────────

export interface Tenant {
  id: string;
  name: string;
  status: string;
  flagship: boolean;
  governance_tier: string;
  created_at: string;
}

export interface CreateTenantForm {
  id: string;
  name: string;
  governance_tier: string;
}

export interface TenantFormErrors {
  id?: string;
  name?: string;
}

// ─── Pure validation (exported for testability) ──────────────────────────────

export function validateTenantForm(form: CreateTenantForm): TenantFormErrors {
  const errors: TenantFormErrors = {};

  if (!form.id.trim()) {
    errors.id = 'Tenant ID is required';
  } else if (!/^[a-z0-9-]+$/.test(form.id.trim())) {
    errors.id = 'Tenant ID must be lowercase letters, numbers, and hyphens only (e.g. mse-bonds)';
  }

  if (!form.name.trim()) {
    errors.name = 'Name is required';
  }

  return errors;
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  if (!iso) return '—';
  try {
    return new Date(iso).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: '2-digit',
    });
  } catch {
    return iso;
  }
}

// ─── Component ────────────────────────────────────────────────────────────────

const EMPTY_FORM: CreateTenantForm = {
  id: '',
  name: '',
  governance_tier: 'STANDARD',
};

export function PlatformAdminPage() {
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [createForm, setCreateForm] = useState<CreateTenantForm>(EMPTY_FORM);
  const [formErrors, setFormErrors] = useState<TenantFormErrors>({});
  const [touched, setTouched] = useState<Partial<Record<keyof CreateTenantForm, boolean>>>({});
  const [submitting, setSubmitting] = useState(false);

  const { showToast } = useToast();

  const { data, refresh, isLoading, error } = usePolling(
    (signal) => fetchTenants(signal),
    30000,
  );

  const rawTenants: Tenant[] = (data as any)?.data ?? (data as any)?.tenants ?? (Array.isArray(data) ? data : []);

  // ─── Derived form validity ─────────────────────────────────────────────────

  const currentErrors = validateTenantForm(createForm);
  const isFormValid = Object.keys(currentErrors).length === 0;

  // ─── Create modal handlers ─────────────────────────────────────────────────

  const openCreateModal = () => {
    setCreateForm(EMPTY_FORM);
    setFormErrors({});
    setTouched({});
    setCreateModalOpen(true);
  };

  const closeCreateModal = () => {
    setCreateModalOpen(false);
    setFormErrors({});
    setTouched({});
  };

  const handleBlur = (field: keyof CreateTenantForm) => {
    setTouched(prev => ({ ...prev, [field]: true }));
    const errs = validateTenantForm(createForm);
    setFormErrors(errs);
  };

  const handleFieldChange = (field: keyof CreateTenantForm, value: string) => {
    const next = { ...createForm, [field]: value };
    setCreateForm(next);
    if (touched[field]) {
      setFormErrors(validateTenantForm(next));
    }
  };

  const handleCreateSubmit = async () => {
    // Mark all fields touched and validate
    setTouched({ id: true, name: true, governance_tier: true });
    const errors = validateTenantForm(createForm);
    if (Object.keys(errors).length > 0) {
      setFormErrors(errors);
      return;
    }

    setSubmitting(true);
    try {
      await createTenant({
        id: createForm.id.trim(),
        name: createForm.name.trim(),
        governance_tier: createForm.governance_tier,
      });
      showToast('Tenant created', 'success');
      closeCreateModal();
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to create tenant', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  // ─── Status change handlers ────────────────────────────────────────────────

  const handleActivate = async (tenant: Tenant) => {
    try {
      await updateTenantStatus(tenant.id, 'ACTIVE');
      showToast(`Tenant ${tenant.id} activated`, 'success');
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to activate tenant', 'error');
    }
  };

  const handleSuspend = async (tenant: Tenant) => {
    try {
      await updateTenantStatus(tenant.id, 'SUSPENDED');
      showToast(`Tenant ${tenant.id} suspended`, 'success');
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to suspend tenant', 'error');
    }
  };

  // ─── Columns ───────────────────────────────────────────────────────────────

  const columns: Column<Tenant>[] = [
    { key: 'id', header: 'ID', sortable: true, mono: true },
    { key: 'name', header: 'Name', sortable: true },
    {
      key: 'status',
      header: 'Status',
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: 'flagship',
      header: 'Flagship',
      render: (row) => row.flagship ? 'Yes' : '—',
    },
    { key: 'governance_tier', header: 'Governance Tier' },
    {
      key: 'created_at',
      header: 'Created',
      sortable: true,
      render: (row) => formatDate(row.created_at),
    },
    {
      key: 'actions',
      header: 'Actions',
      render: (row) => (
        <div className={styles.actionBtns}>
          {(row.status === 'SUSPENDED' || row.status === 'ONBOARDING') && (
            <button
              className={styles.activateBtn}
              onClick={() => handleActivate(row)}
              aria-label={`Activate tenant ${row.id}`}
            >
              Activate
            </button>
          )}
          {row.status === 'ACTIVE' && (
            <button
              className={styles.suspendBtn}
              onClick={() => handleSuspend(row)}
              aria-label={`Suspend tenant ${row.id}`}
            >
              Suspend
            </button>
          )}
        </div>
      ),
    },
  ];

  // ─── Render ────────────────────────────────────────────────────────────────

  return (
    <div>
      <h1>Platform — Tenant Management</h1>

      <div className={styles.toolbar}>
        <button
          className={styles.createBtn}
          onClick={openCreateModal}
          aria-label="Create new tenant"
        >
          Create Tenant
        </button>
      </div>

      {error && !isLoading ? (
        <div className={styles.errorState} role="alert" aria-live="assertive">
          <span className={styles.errorMessage}>
            Unable to load tenants. Please check your connection and try again.
          </span>
          <button className={styles.retryBtn} onClick={refresh}>
            Retry
          </button>
        </div>
      ) : (
        <DataGrid
          columns={columns}
          data={rawTenants}
          keyField="id"
          emptyMessage="No tenants found. Create the first tenant to get started."
          exportFilename="platform-tenants"
          loading={isLoading}
        />
      )}

      {/* ─── Create Tenant Modal ──────────────────────────────────────────── */}
      {createModalOpen && (
        <div className={styles.overlay} onClick={closeCreateModal}>
          <div
            className={styles.modal}
            onClick={e => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-label="Create tenant"
            aria-describedby="create-tenant-desc"
          >
            <h3 id="create-tenant-desc">Create Tenant</h3>

            <label className={styles.formLabel}>
              Tenant ID *
              <input
                type="text"
                value={createForm.id}
                onChange={e => handleFieldChange('id', e.target.value)}
                onBlur={() => handleBlur('id')}
                className={styles.formInput}
                aria-invalid={!!(touched.id && formErrors.id)}
                aria-describedby={formErrors.id ? 'id-error' : 'id-hint'}
                placeholder="e.g. mse-bonds"
                pattern="[a-z0-9-]+"
                required
                autoFocus
              />
              <span id="id-hint" className={styles.fieldHint}>
                Lowercase letters, numbers, and hyphens only
              </span>
              {touched.id && formErrors.id && (
                <span id="id-error" className={styles.fieldError} role="alert">
                  {formErrors.id}
                </span>
              )}
            </label>

            <label className={styles.formLabel}>
              Name *
              <input
                type="text"
                value={createForm.name}
                onChange={e => handleFieldChange('name', e.target.value)}
                onBlur={() => handleBlur('name')}
                className={styles.formInput}
                aria-invalid={!!(touched.name && formErrors.name)}
                aria-describedby={formErrors.name ? 'name-error' : undefined}
                placeholder="e.g. Mongolian Stock Exchange"
                required
              />
              {touched.name && formErrors.name && (
                <span id="name-error" className={styles.fieldError} role="alert">
                  {formErrors.name}
                </span>
              )}
            </label>

            <label className={styles.formLabel}>
              Governance Tier
              <select
                value={createForm.governance_tier}
                onChange={e => handleFieldChange('governance_tier', e.target.value)}
                className={styles.formInput}
                aria-label="Governance tier"
              >
                <option value="STANDARD">STANDARD</option>
                <option value="FLAGSHIP">FLAGSHIP</option>
                <option value="SANDBOX">SANDBOX</option>
              </select>
            </label>

            <div className={styles.modalActions}>
              <button
                onClick={closeCreateModal}
                className={styles.cancelBtn}
                type="button"
                disabled={submitting}
              >
                Cancel
              </button>
              <button
                onClick={handleCreateSubmit}
                className={styles.saveBtn}
                type="button"
                disabled={!isFormValid || submitting}
                aria-disabled={!isFormValid || submitting}
              >
                {submitting ? 'Creating…' : 'Create'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
