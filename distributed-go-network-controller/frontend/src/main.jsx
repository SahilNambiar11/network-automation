import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { api } from './api';
import './styles.css';

const sampleConfig = `devices:
  - name: core-router
    type: router

  - name: access-switch
    type: switch

vlans:
  - id: 10
    name: engineering
    subnet: 10.10.0.0/24

  - id: 20
    name: guest
    subnet: 10.20.0.0/24

firewall_rules:
  - source: guest
    destination: engineering
    port: 22
    action: deny
`;

const initialState = {
  health: null,
  deployments: [],
  jobs: [],
  agents: [],
  devices: [],
  drift: [],
  driftSummary: { devices_checked: 0, devices_with_drift: 0 },
};

function App() {
  const [data, setData] = useState(initialState);
  const [configText, setConfigText] = useState(sampleConfig);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const [validationResult, setValidationResult] = useState(null);
  const [deploymentResult, setDeploymentResult] = useState(null);
  const [actionLoading, setActionLoading] = useState('');

  const refreshDashboard = useCallback(async ({ quiet = false } = {}) => {
    if (quiet) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError('');

    try {
      const [
        health,
        deployments,
        jobs,
        agents,
        devices,
        drift,
        driftSummary,
      ] = await Promise.all([
        api.health(),
        api.deployments(),
        api.jobs(),
        api.agents(),
        api.devices(),
        api.drift(),
        api.driftSummary(),
      ]);

      setData({
        health,
        deployments,
        jobs,
        agents,
        devices,
        drift,
        driftSummary,
      });
    } catch (err) {
      setError(err.message || 'Failed to refresh dashboard data');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    refreshDashboard();
    const interval = window.setInterval(() => {
      refreshDashboard({ quiet: true });
    }, 5000);

    return () => window.clearInterval(interval);
  }, [refreshDashboard]);

  const overviewCards = useMemo(() => {
    const agents = asArray(data.agents);
    const deployments = asArray(data.deployments);
    const jobs = asArray(data.jobs);
    const healthyAgents = agents.filter((agent) => agent.status === 'healthy').length;
    const unhealthyAgents = agents.filter((agent) => agent.status === 'unhealthy').length;

    return [
      {
        label: 'API Health',
        value: data.health?.status || 'unknown',
        tone: data.health?.status === 'ok' ? 'good' : 'warning',
      },
      { label: 'Deployments', value: deployments.length },
      { label: 'Jobs', value: jobs.length },
      { label: 'Healthy Agents', value: healthyAgents, tone: 'good' },
      {
        label: 'Unhealthy Agents',
        value: unhealthyAgents,
        tone: unhealthyAgents > 0 ? 'bad' : 'good',
      },
      { label: 'Devices Checked', value: data.driftSummary.devices_checked ?? 0 },
      {
        label: 'Devices With Drift',
        value: data.driftSummary.devices_with_drift ?? 0,
        tone: data.driftSummary.devices_with_drift > 0 ? 'bad' : 'good',
      },
    ];
  }, [data]);

  async function handleValidate() {
    setActionLoading('validate');
    setValidationResult(null);
    setDeploymentResult(null);
    setError('');

    try {
      const result = await api.validateConfig(configText);
      setValidationResult(result);
    } catch (err) {
      setError(err.message || 'Validation request failed');
    } finally {
      setActionLoading('');
    }
  }

  async function handleDeploy() {
    setActionLoading('deploy');
    setValidationResult(null);
    setDeploymentResult(null);
    setError('');

    try {
      const result = await api.createDeployment(configText);
      if (result.valid === false) {
        setValidationResult(result);
        return;
      }
      setDeploymentResult(result);
      await refreshDashboard({ quiet: true });
    } catch (err) {
      setError(err.message || 'Deployment request failed');
    } finally {
      setActionLoading('');
    }
  }

  async function handleMutateDevice(deviceName, mutation) {
    setActionLoading(`${deviceName}:${Object.keys(mutation)[0]}`);
    setError('');

    try {
      await api.mutateDevice(deviceName, mutation);
      await refreshDashboard({ quiet: true });
    } catch (err) {
      setError(err.message || `Failed to mutate ${deviceName}`);
    } finally {
      setActionLoading('');
    }
  }

  return (
    <main className="app-shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">Distributed Go Network Controller</p>
          <h1>Automation Dashboard</h1>
        </div>
        <button className="button secondary" onClick={() => refreshDashboard()} disabled={loading}>
          {refreshing ? 'Refreshing' : 'Refresh'}
        </button>
      </header>

      {error && <div className="banner error">{error}</div>}

      <section className="section" aria-labelledby="overview-title">
        <SectionHeader title="Overview" description="Live controller state from the API." />
        <div className="overview-grid">
          {overviewCards.map((card) => (
            <article className={`metric-card ${card.tone || ''}`} key={card.label}>
              <span>{card.label}</span>
              <strong>{card.value}</strong>
            </article>
          ))}
        </div>
      </section>

      <section className="section split" aria-labelledby="deploy-title">
        <div>
          <SectionHeader title="Deploy Config" description="Validate or deploy network YAML." />
          <textarea
            className="config-editor"
            value={configText}
            onChange={(event) => setConfigText(event.target.value)}
            spellCheck="false"
          />
          <div className="actions">
            <button className="button secondary" onClick={handleValidate} disabled={actionLoading !== ''}>
              {actionLoading === 'validate' ? 'Validating' : 'Validate'}
            </button>
            <button className="button primary" onClick={handleDeploy} disabled={actionLoading !== ''}>
              {actionLoading === 'deploy' ? 'Deploying' : 'Deploy'}
            </button>
          </div>
        </div>
        <DeployResult validationResult={validationResult} deploymentResult={deploymentResult} />
      </section>

      <section className="section" aria-labelledby="deployments-title">
        <SectionHeader title="Deployments" description="Desired configuration submissions." />
        <DataTable
          columns={['id', 'status', 'created_at', 'completed_at']}
          rows={data.deployments}
          renderCell={(row, column) =>
            column === 'status' ? <StatusBadge status={row.status} /> : formatValue(row[column])
          }
          empty="No deployments yet."
        />
      </section>

      <section className="section" aria-labelledby="jobs-title">
        <SectionHeader title="Jobs" description="Per-device deployment execution." />
        <DataTable
          columns={[
            'id',
            'deployment_id',
            'device_name',
            'device_type',
            'status',
            'attempts',
            'max_attempts',
            'claimed_by',
            'started_at',
            'completed_at',
            'error',
          ]}
          rows={data.jobs}
          renderCell={(row, column) =>
            column === 'status' ? <StatusBadge status={row.status} /> : formatValue(row[column])
          }
          empty="No jobs yet."
        />
      </section>

      <section className="section" aria-labelledby="agents-title">
        <SectionHeader title="Agents" description="Worker heartbeat and capacity state." />
        <DataTable
          columns={['id', 'hostname', 'status', 'last_heartbeat', 'active_jobs']}
          rows={data.agents}
          renderCell={(row, column) =>
            column === 'status' ? <StatusBadge status={row.status} /> : formatValue(row[column])
          }
          empty="No agents have checked in yet."
        />
      </section>

      <section className="section" aria-labelledby="devices-title">
        <SectionHeader title="Devices" description="Actual device state stored after successful jobs." />
        <DataTable
          columns={['device_name', 'device_type', 'updated_at', 'vlans', 'firewall_rules', 'actions']}
          rows={data.devices}
          renderCell={(row, column) => {
            if (column === 'vlans') return vlanIDs(row.actual_config).join(', ') || 'none';
            if (column === 'firewall_rules') return firewallRuleCount(row.actual_config);
            if (column === 'actions') {
              return (
                <div className="row-actions">
                  <button
                    className="button compact"
                    onClick={() => handleMutateDevice(row.device_name, { remove_vlan: 10 })}
                    disabled={actionLoading !== ''}
                  >
                    Remove VLAN 10
                  </button>
                  <button
                    className="button compact danger"
                    onClick={() => handleMutateDevice(row.device_name, { clear_firewall_rules: true })}
                    disabled={actionLoading !== ''}
                  >
                    Clear Firewall Rules
                  </button>
                </div>
              );
            }
            return formatValue(row[column]);
          }}
          empty="No device states yet."
        />
      </section>

      <section className="section" aria-labelledby="drift-title">
        <SectionHeader title="Drift" description="Desired versus actual configuration comparison." />
        <DataTable
          columns={[
            'device_name',
            'drift',
            'missing_vlans',
            'extra_vlans',
            'missing_firewall_rules',
            'extra_firewall_rules',
          ]}
          rows={data.drift}
          renderCell={(row, column) => {
            if (column === 'drift') {
              return <StatusBadge status={row.drift ? 'drift' : 'no drift'} />;
            }
            if (column === 'missing_firewall_rules' || column === 'extra_firewall_rules') {
              return asArray(row[column]).length;
            }
            if (column === 'missing_vlans' || column === 'extra_vlans') {
              return asArray(row[column]).join(', ') || 'none';
            }
            return formatValue(row[column]);
          }}
          empty="No drift reports yet."
        />
      </section>

      <section className="section" aria-labelledby="observability-title">
        <SectionHeader title="Observability Links" description="Open the metrics and dashboard tools." />
        <div className="link-row">
          <a className="button secondary" href="http://localhost:9090" target="_blank" rel="noreferrer">
            Prometheus
          </a>
          <a className="button secondary" href="http://localhost:3000" target="_blank" rel="noreferrer">
            Grafana
          </a>
        </div>
      </section>

      {loading && <div className="loading">Loading dashboard data...</div>}
    </main>
  );
}

function SectionHeader({ title, description }) {
  return (
    <div className="section-header">
      <h2>{title}</h2>
      <p>{description}</p>
    </div>
  );
}

function DeployResult({ validationResult, deploymentResult }) {
  if (!validationResult && !deploymentResult) {
    return (
      <aside className="result-panel">
        <h3>Result</h3>
        <p className="muted">Validation and deployment responses appear here.</p>
      </aside>
    );
  }

  return (
    <aside className="result-panel">
      <h3>Result</h3>
      {validationResult && (
        <div>
          <StatusBadge status={validationResult.valid ? 'valid' : 'invalid'} />
          {asArray(validationResult.errors).length > 0 ? (
            <ul className="error-list">
              {asArray(validationResult.errors).map((item, index) => (
                <li key={`${item.field}-${index}`}>
                  <strong>{item.field}</strong>: {item.message}
                </li>
              ))}
            </ul>
          ) : (
            <p className="muted">Configuration is valid.</p>
          )}
        </div>
      )}
      {deploymentResult && (
        <div className="result-details">
          <p>
            <span>deployment_id</span>
            <code>{deploymentResult.deployment_id}</code>
          </p>
          <p>
            <span>jobs_created</span>
            <strong>{deploymentResult.jobs_created}</strong>
          </p>
        </div>
      )}
    </aside>
  );
}

function DataTable({ columns, rows, renderCell, empty }) {
  const safeRows = asArray(rows);

  if (!safeRows.length) {
    return <div className="empty-state">{empty}</div>;
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column}>{column.replaceAll('_', ' ')}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {safeRows.map((row, index) => (
            <tr key={row.id || row.device_name || `${row.deployment_id}-${index}`}>
              {columns.map((column) => (
                <td key={column}>{renderCell(row, column)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function StatusBadge({ status }) {
  const normalized = String(status || 'unknown').toLowerCase();
  return <span className={`badge ${normalized.replaceAll(' ', '-')}`}>{status}</span>;
}

function vlanIDs(actualConfig) {
  return asArray(actualConfig?.vlans)
    .map((vlan) => vlan.id)
    .filter((id) => id !== undefined);
}

function firewallRuleCount(actualConfig) {
  return asArray(actualConfig?.firewall_rules).length;
}

function formatValue(value) {
  if (value === null || value === undefined || value === '') return 'none';
  if (typeof value === 'string' && value.includes('T')) return new Date(value).toLocaleString();
  return String(value);
}

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

createRoot(document.getElementById('root')).render(<App />);
