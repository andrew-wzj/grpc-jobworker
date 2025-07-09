let allJobs = [];
let currentFilter = 'all';

async function loadJobs() {
    try {
      const res = await fetch("/list");
      const jobs = await res.json();
  
      allJobs = jobs.map(job => ({
        ID: job.id || job.ID || "N/A",
        Name: job.name || job.Name || "Untitled Job",
        Status: job.status || job.Status || "pending",
        StartTime: job.start_time || job.StartTime || new Date().toLocaleString(),
        Duration: job.duration || job.Duration || "---",
        Branch: job.branch || job.Branch || "main"
      }));
  
      updateStats();
      renderJobs();
    } catch (e) {
      console.error("Failed to load jobs:", e);
      document.getElementById("jobContainer").innerHTML = `
        <div class="empty-state">
          <h3>Failed to load jobs</h3>
          <p>Please check your connection and try again.</p>
        </div>
      `;
    }
  }
  

function updateStats() {
  const stats = allJobs.reduce((acc, job) => {
    acc.total++;
    acc[job.Status] = (acc[job.Status] || 0) + 1;
    return acc;
  }, { total: 0, success: 0, failed: 0, running: 0, pending: 0 });

  document.getElementById('totalJobs').textContent = stats.total;
  document.getElementById('successJobs').textContent = stats.success || 0;
  document.getElementById('failedJobs').textContent = stats.failed || 0;
  document.getElementById('runningJobs').textContent = stats.running || 0;
}

function renderJobs() {
  const container = document.getElementById("jobContainer");
  const filteredJobs = filterJobs();

  if (filteredJobs.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <h3>No jobs found</h3>
        <p>Try adjusting your search or filter criteria.</p>
      </div>
    `;
    return;
  }

  container.innerHTML = filteredJobs.map(job => `
    <div class="card">
      <div class="card-header">
        <div class="job-id">${job.ID}</div>
        <div class="status-badge status-${job.Status}">${job.Status}</div>
      </div>
      <div class="job-name">${job.Name}</div>
      <div class="job-details">
        <div class="detail-item">
          <div class="detail-label">Start Time</div>
          <div class="detail-value">${job.StartTime || '---'}</div>
        </div>
        <div class="detail-item">
          <div class="detail-label">Duration</div>
          <div class="detail-value">${job.Duration}</div>
        </div>
        <div class="detail-item">
          <div class="detail-label">Branch</div>
          <div class="detail-value">${job.Branch}</div>
        </div>
        <div class="detail-item">
          <div class="detail-label">Status</div>
          <div class="detail-value">${job.Status}</div>
        </div>
      </div>
      <div class="card-actions">
        <a class="action-btn btn-secondary" href="/details/${job.ID}">Details</a>
        <a class="action-btn btn-primary" href="/log/${job.ID}" target="_blank">View Log</a>
        <button class="action-btn btn-danger" onclick="deleteJob('${job.ID}')">🗑️ Delete</button>
      </div>
    </div>
  `).join('');
}

function filterJobs() {
  let filtered = allJobs;

  if (currentFilter !== 'all') {
    filtered = filtered.filter(job => job.Status === currentFilter);
  }

  const searchTerm = document.getElementById('searchInput').value.toLowerCase();
  if (searchTerm) {
    filtered = filtered.filter(job =>
      job.Name.toLowerCase().includes(searchTerm) ||
      job.ID.toLowerCase().includes(searchTerm)
    );
  }

  return filtered;
}

async function deleteJob(id) {
  const confirmDelete = confirm(`确认删除任务 ${id} 吗？此操作不可恢复。`);
  if (!confirmDelete) return;

  try {
    const res = await fetch(`/delete/${id}`, {
      method: 'DELETE'
    });

    if (res.ok) {
      alert('✅ 删除成功');
      await loadJobs();
    } else {
      const error = await res.json();
      alert('❌ 删除失败：' + (error.error || '未知错误'));
    }
  } catch (err) {
    console.error('删除失败:', err);
    alert('⚠️ 网络错误或服务器异常');
  }
}

// Initial Load & Event Listeners
window.addEventListener('DOMContentLoaded', () => {
  loadJobs();

  document.getElementById('searchInput').addEventListener('input', renderJobs);

  document.querySelectorAll('.filter-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
      e.target.classList.add('active');
      currentFilter = e.target.dataset.filter;
      renderJobs();
    });
  });
});
