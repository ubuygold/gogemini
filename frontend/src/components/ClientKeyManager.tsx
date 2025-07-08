import { useState, useEffect } from 'react';

type APIKey = {
  ID: number;
  Key: string;
  UsageCount: number;
  Status: string;
  Permissions: string;
  RateLimit: number;
  ExpiresAt: string;
};

type ClientKeyManagerProps = {
  password: string;
};

function ClientKeyManager({ password }: ClientKeyManagerProps) {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [newKey, setNewKey] = useState('');

  useEffect(() => {
    fetchKeys();
  }, []);

  const fetchKeys = async () => {
    const response = await fetch('/admin/client-keys', {
      headers: {
        Authorization: `Basic ${btoa(`admin:${password}`)}`,
      },
    });
    const data = await response.json();
    setKeys(data);
  };

  const createKey = async () => {
    await fetch('/admin/client-keys', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Basic ${btoa(`admin:${password}`)}`,
      },
      body: JSON.stringify({ Key: newKey, Permissions: 'all' }),
    });
    setNewKey('');
    fetchKeys();
  };

  const deleteKey = async (id: number) => {
    const response = await fetch(`/admin/client-keys/${id}`, {
      method: 'DELETE',
      headers: {
        Authorization: `Basic ${btoa(`admin:${password}`)}`,
      },
    });
    if (response.ok) {
      fetchKeys();
    } else {
      const error = await response.json();
      alert(`Failed to delete key: ${error.error}`);
    }
  };

  const resetKey = async (id: number) => {
    if (!confirm(`Are you sure you want to reset the usage count for key ID ${id}?`)) {
      return;
    }
    const response = await fetch(`/admin/client-keys/${id}/reset`, {
      method: 'POST',
      headers: {
        Authorization: `Basic ${btoa(`admin:${password}`)}`,
      },
    });
    if (response.ok) {
      alert('Key usage count reset successfully.');
      fetchKeys();
    } else {
      const error = await response.json();
      alert(`Failed to reset key: ${error.error}`);
    }
  };

  const generateRandomKey = () => {
    const randomString =
      'sk-' +
      Array.from({ length: 32 }, () =>
        'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'.charAt(
          Math.floor(Math.random() * 62)
        )
      ).join('');
    setNewKey(randomString);
  };

  return (
    <div className="p-4 bg-base-100">
      <div className="card bg-base-200 shadow-xl mb-4">
        <div className="card-body">
          <h2 className="card-title">Add New Client Key</h2>
          <div className="flex items-center gap-4">
            <div className="form-control flex-grow">
              <label className="input input-bordered flex items-center gap-2">
                Key
                <input
                  type="text"
                  className="grow"
                  placeholder="sk-..."
                  value={newKey}
                  onChange={(e) => setNewKey(e.target.value)}
                />
              </label>
            </div>
            <div className="card-actions flex gap-2">
              <button onClick={generateRandomKey} className="btn">
                Generate Random
              </button>
              <button onClick={createKey} className="btn btn-primary">
                Add Key
              </button>
            </div>
          </div>
        </div>
      </div>

      <div className="card bg-base-200 shadow-xl">
        <div className="card-body">
          <h2 className="card-title">Client Keys</h2>
          <div className="overflow-x-auto">
            <table className="table w-full">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Key</th>
                  <th>Usage Count</th>
                  <th>Status</th>
                  <th>Permissions</th>
                  <th>Rate Limit</th>
                  <th>Expires At</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {keys.map((key) => (
                  <tr key={key.ID} className="hover">
                    <td>{key.ID}</td>
                    <td>{key.Key}</td>
                    <td>{key.UsageCount}</td>
                    <td>
                      <span
                        className={`badge ${
                          key.Status === 'active'
                            ? 'badge-success'
                            : 'badge-error'
                        }`}
                      >
                        {key.Status}
                      </span>
                    </td>
                    <td>{key.Permissions}</td>
                    <td>{key.RateLimit}</td>
                    <td>{new Date(key.ExpiresAt).toLocaleString()}</td>
                    <td className="flex gap-2">
                      <button
                        onClick={() => resetKey(key.ID)}
                        className="btn btn-warning btn-sm"
                      >
                        Reset Usage
                      </button>
                      <button
                        onClick={() => deleteKey(key.ID)}
                        className="btn btn-error btn-sm"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}

export default ClientKeyManager;