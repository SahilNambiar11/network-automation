const configuredBaseURL = import.meta.env.VITE_API_BASE_URL || '/api';
const API_BASE_URL = configuredBaseURL.replace(/\/$/, '');

async function request(path, options = {}) {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...options,
    headers: {
      ...(options.headers || {}),
    },
  });

  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }

  const contentType = response.headers.get('content-type') || '';
  if (!contentType.includes('application/json')) {
    return response.text();
  }

  return response.json();
}

export const api = {
  health: () => request('/health'),
  validateConfig: (yaml) =>
    request('/validate', {
      method: 'POST',
      body: yaml,
    }),
  createDeployment: (yaml) =>
    request('/deployments', {
      method: 'POST',
      body: yaml,
    }),
  deployments: () => request('/deployments'),
  jobs: () => request('/jobs'),
  agents: () => request('/agents'),
  devices: () => request('/devices'),
  drift: () => request('/drift'),
  driftSummary: () => request('/drift/summary'),
  mutateDevice: (deviceName, mutation) =>
    request(`/devices/${encodeURIComponent(deviceName)}/mutate`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(mutation),
    }),
};
