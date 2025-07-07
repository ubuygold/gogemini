import { useState, useEffect, useCallback } from 'react';
import { ElMessage } from 'element-plus';
import 'element-plus/dist/index.css';

type GeminiKey = {
  ID: number;
  Key: string;
  Status: string;
  FailureCount: number;
  UsageCount: number;
};

type GeminiKeyManagerProps = {
  password: string;
};

function GeminiKeyManager({ password }: GeminiKeyManagerProps) {
  const [keys, setKeys] = useState<GeminiKey[]>([]);
  const [newKeys, setNewKeys] = useState('');
  const [selectedKeys, setSelectedKeys] = useState<number[]>([]);
  
  // Filtering and Pagination state
  const [statusFilter, setStatusFilter] = useState('all');
  const [failureCountFilter, setFailureCountFilter] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [limit, setLimit] = useState(10); // Items per page

  const fetchKeys = useCallback(async (page: number) => {
    const params = new URLSearchParams({
      page: page.toString(),
      limit: limit.toString(),
      status: statusFilter,
      minFailureCount: failureCountFilter,
    });
    try {
      const response = await fetch(`/admin/gemini-keys?${params.toString()}`, {
        headers: {
          Authorization: `Basic ${btoa(`admin:${password}`)}`,
        },
      });
      if (!response.ok) {
        throw new Error('Failed to fetch keys');
      }
      const data = await response.json();
      setKeys(data.keys || []);
      setTotalPages(Math.ceil(data.total / limit) || 1);
      setCurrentPage(page);
    } catch (error) {
      console.error("Fetch keys error:", error);
      // Optionally, handle the error in the UI
    }
  }, [password, limit, statusFilter, failureCountFilter]);

  // Fetch keys when page or filters change
  useEffect(() => {
    fetchKeys(currentPage);
  }, [currentPage, fetchKeys, limit]);
  
  const handleFilterChange = () => {
    setCurrentPage(1);
    fetchKeys(1);
  };

  const createKeys = async () => {
    const keysToAdd = newKeys.split('\n').filter((k) => k.trim() !== '');
    if (keysToAdd.length === 0) return;
    
    await fetch('/admin/gemini-keys/batch', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Basic ${btoa(`admin:${password}`)}`,
      },
      body: JSON.stringify({ keys: keysToAdd }),
    });
    setNewKeys('');
    fetchKeys(1); // Refresh to page 1
  };

  const deleteSelectedKeys = async () => {
    if (selectedKeys.length === 0) return;

    const response = await fetch('/admin/gemini-keys/batch', {
      method: 'DELETE',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Basic ${btoa(`admin:${password}`)}`,
      },
      body: JSON.stringify({ ids: selectedKeys }),
    });

    if (response.ok) {
      setSelectedKeys([]);
      // Refresh current page, but if it becomes empty, go to the previous one
      if (keys.length === selectedKeys.length && currentPage > 1) {
        fetchKeys(currentPage - 1);
      } else {
        fetchKeys(currentPage);
      }
    } else {
      const error = await response.json();
      alert(`Failed to delete keys: ${error.error}`);
    }
  };

  const handleSelectKey = (id: number) => {
    setSelectedKeys((prev) =>
      prev.includes(id) ? prev.filter((k) => k !== id) : [...prev, id]
    );
  };

  const handleTestKey = async (id: number) => {
    try {
      const response = await fetch(`/admin/gemini-keys/${id}/test`, {
        method: 'POST',
        headers: {
          Authorization: `Basic ${btoa(`admin:${password}`)}`,
        },
      });
      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.error || 'Failed to test key');
      }
      ElMessage.success(`Key ID ${id} test passed.`);
      fetchKeys(currentPage); // Refresh to show updated status
    } catch (error: any) {
      ElMessage.error(`Key ID ${id} test failed: ${error.message}`);
    }
  };

  const handleTestAllKeys = async () => {
    try {
      const response = await fetch('/admin/gemini-keys/test', {
        method: 'POST',
        headers: {
          Authorization: `Basic ${btoa(`admin:${password}`)}`,
        },
      });
      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || 'Failed to start batch test');
      }
      ElMessage.info('Batch key test initiated in the background. Refresh later to see results.');
    } catch (error: any) {
      ElMessage.error(`Failed to start batch test: ${error.message}`);
    }
  };

  const handleActivateKey = async (id: number) => {
    try {
      const response = await fetch(`/admin/gemini-keys/${id}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Basic ${btoa(`admin:${password}`)}`,
        },
        body: JSON.stringify({ status: 'active' }),
      });
      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || 'Failed to activate key');
      }
      ElMessage.success(`Key ID ${id} has been activated.`);
      fetchKeys(currentPage); // Refresh the list
    } catch (error: any) {
      ElMessage.error(`Failed to activate key: ${error.message}`);
    }
  };

  return (
    <div className="p-4 bg-base-100">
      <div className="card bg-base-200 shadow-xl mb-4">
        <div className="card-body">
          <h2 className="card-title">Add New Gemini Keys</h2>
          <textarea
            value={newKeys}
            onChange={(e) => setNewKeys(e.target.value)}
            className="textarea textarea-bordered w-full"
            placeholder="Enter one key per line to add in batch"
            rows={4}
          />
          <div className="card-actions justify-end">
            <button onClick={createKeys} className="btn btn-primary">
              Add Keys
            </button>
          </div>
        </div>
      </div>

      <div className="card bg-base-200 shadow-xl">
        <div className="card-body">
          <div className="flex justify-between items-center mb-4">
            <h2 className="card-title">Gemini Keys</h2>
            <button className="btn btn-ghost btn-sm" onClick={() => fetchKeys(currentPage)}>
              {/* @ts-ignore */}
              <svg t="1751844619661" class="icon" viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="1471" width="16" height="16"><path d="M512 166.4c-70.4 0-134.4 19.2-192 57.6L294.4 185.6C281.6 166.4 256 172.8 249.6 192L204.8 332.8C204.8 345.6 217.6 364.8 230.4 364.8l147.2-6.4c19.2 0 32-25.6 19.2-38.4L364.8 281.6l0 0 0-6.4C403.2 243.2 460.8 230.4 512 230.4c153.6 0 281.6 128 281.6 281.6s-128 281.6-281.6 281.6-281.6-128-281.6-281.6c0-19.2-12.8-32-32-32S166.4 492.8 166.4 512c0 192 153.6 345.6 345.6 345.6S857.6 704 857.6 512 704 166.4 512 166.4z" fill="#1296db" p-id="1472"></path></svg>
              Refresh
            </button>
          </div>
          <div className="mb-4 p-4 border rounded-md">
            <h3 className="font-bold mb-2">Filters</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="label">
                  <span className="label-text">Filter by Status</span>
                </label>
                <select
                  className="select select-bordered w-full"
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                >
                  <option value="all">All</option>
                  <option value="active">Active</option>
                  <option value="disabled">Disabled</option>
                </select>
              </div>
              <div>
                <label className="label">
                  <span className="label-text">Min Failure Count</span>
                </label>
                <input
                  type="number"
                  placeholder="0"
                  className="input input-bordered w-full"
                  value={failureCountFilter}
                  onChange={(e) => setFailureCountFilter(e.target.value)}
                  onBlur={handleFilterChange}
                />
              </div>
               <div>
                  <label className="label">
                    <span className="label-text">Items Per Page</span>
                  </label>
                  <select
                    className="select select-bordered w-full"
                    value={limit}
                    onChange={(e) => {
                      setLimit(Number(e.target.value));
                      setCurrentPage(1); // Reset to first page
                    }}
                  >
                    <option value={10}>10</option>
                    <option value={25}>25</option>
                    <option value={50}>50</option>
                    <option value={100}>100</option>
                  </select>
                </div>
            </div>
          </div>

          <div className="mb-4 flex items-center justify-between">
            <button
              onClick={deleteSelectedKeys}
              className="btn btn-error"
              disabled={selectedKeys.length === 0}
            >
              Delete Selected ({selectedKeys.length})
            </button>
             <button onClick={handleTestAllKeys} className="btn btn-accent">
              Test All Keys
            </button>
            <button
              className="btn"
              onClick={() => {
                if (selectedKeys.length === keys.length) {
                  setSelectedKeys([]);
                } else {
                  setSelectedKeys(keys.map((k) => k.ID));
                }
              }}
            >
              {selectedKeys.length === keys.length
                ? 'Unselect All on Page'
                : 'Select All on Page'}
            </button>
          </div>

          <div className="overflow-x-auto">
            <table className="table w-full">
              <thead>
                <tr>
                  <th>
                    <label>
                      <input
                        type="checkbox"
                        className="checkbox"
                        onChange={(e) => {
                          if (e.target.checked) {
                            setSelectedKeys(keys.map((k) => k.ID));
                          } else {
                            setSelectedKeys([]);
                          }
                        }}
                        checked={
                          keys.length > 0 &&
                          selectedKeys.length === keys.length
                        }
                      />
                    </label>
                  </th>
                  <th>ID</th>
                  <th>Key</th>
                  <th>Status</th>
                  <th>Failure Count</th>
                  <th>Usage Count</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {keys.map((key) => (
                  <tr key={key.ID} className="hover">
                    <th>
                      <label>
                        <input
                          type="checkbox"
                          className="checkbox"
                          checked={selectedKeys.includes(key.ID)}
                          onChange={() => handleSelectKey(key.ID)}
                        />
                      </label>
                    </th>
                    <td>{key.ID}</td>
                    <td>{key.Key}</td>
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
                    <td>{key.FailureCount}</td>
                    <td>{key.UsageCount}</td>
                    <td className="space-x-2">
                      <button
                        onClick={() => handleTestKey(key.ID)}
                        className="btn btn-xs btn-outline btn-info"
                      >
                        Test
                      </button>
                      {key.Status === 'disabled' && (
                        <button
                          onClick={() => handleActivateKey(key.ID)}
                          className="btn btn-xs btn-outline btn-success"
                        >
                          Activate
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex justify-center mt-4">
            <div className="join">
              <button
                className="join-item btn"
                onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                disabled={currentPage === 1}
              >
                «
              </button>
              <button className="join-item btn">
                Page {currentPage} of {totalPages}
              </button>
              <button
                className="join-item btn"
                onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                disabled={currentPage === totalPages}
              >
                »
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

export default GeminiKeyManager;