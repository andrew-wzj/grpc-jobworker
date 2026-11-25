'use strict';

// All job data is inserted with textContent, never innerHTML, so job names
// and commands cannot inject markup.

const $ = (id) => document.getElementById(id);
const ACTIVE = new Set(['pending', 'running']);
let jobs = [];
let filter = 'all';
let logAbort = null;

function token() {
  try { return sessionStorage.getItem('jobworker-token') || ''; } catch { return ''; }
}

async function api(path, options = {}) {
  const headers = { ...(options.headers || {}) };
  if (token()) headers.Authorization = 'Bearer ' + token();
  const res = await fetch(path, { ...options, headers });
  if (res.status === 401) {
    $('tokenPanel').hidden = false;
    throw new Error('Authentication required');
  }
  if (!res.ok) {
    let msg = res.statusText;
    try { msg = (await res.json()).error || msg; } catch { /* not JSON */ }
    throw new Error(msg);
  }
  return res;
}

function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  Object.assign(node, props);
  for (const c of children) node.append(c);
  return node;
}

function formatTime(iso) {
  return iso ? new Date(iso).toLocaleString() : '—';
}

function formatDuration(job) {
  if (!job.started_at) return '—';
  const end = job.finished_at ? new Date(job.finished_at) : new Date();
  const s = (end - new Date(job.started_at)) / 1000;
  if (s < 60) return s.toFixed(1) + 's';
  const m = Math.floor(s / 60);
  return m < 60 ? `${m}m ${Math.round(s % 60)}s` : `${Math.floor(m / 60)}h ${m % 60}m`;
}

function matches(job) {
  if (filter === 'active' && !ACTIVE.has(job.status)) return false;
  if (filter === 'stopped' && !['stopped', 'interrupted'].includes(job.status)) return false;
  if (!['all', 'active', 'stopped'].includes(filter) && job.status !== filter) return false;
  const q = $('search').value.trim().toLowerCase();
  return !q || [job.id, job.name, job.cmd].some((s) => s.toLowerCase().includes(q));
}

function renderStats() {
  const count = (pred) => jobs.filter(pred).length;
  const items = [
    ['Total', jobs.length],
    ['Active', count((j) => ACTIVE.has(j.status))],
    ['Completed', count((j) => j.status === 'completed')],
    ['Failed', count((j) => j.status === 'failed')],
  ];
  $('stats').replaceChildren(...items.map(([label, n]) =>
    el('div', {}, el('dt', { textContent: label }), el('dd', { textContent: String(n) }))));
}

function renderJobs() {
  const rows = jobs.filter(matches).map((job) => {
    const actions = el('td', { className: 'actions' },
      el('button', { className: 'ghost small', textContent: 'Logs', onclick: () => openLog(job) }));
    if (ACTIVE.has(job.status)) {
      actions.append(el('button', { className: 'danger small', textContent: 'Stop', onclick: () => stopJob(job) }));
    } else {
      actions.append(el('button', { className: 'ghost small', textContent: 'Delete', onclick: () => deleteJob(job) }));
    }
    const exit = ['completed', 'failed'].includes(job.status) ? String(job.exit_code) : '—';
    return el('tr', {},
      el('td', {}, el('code', { textContent: job.id })),
      el('td', { className: 'name', title: job.error || '' },
        document.createTextNode(job.name), el('small', { textContent: job.cmd })),
      el('td', {}, el('span', { className: 'badge ' + job.status, textContent: job.status })),
      el('td', { textContent: formatTime(job.created_at) }),
      el('td', { textContent: formatDuration(job) }),
      el('td', { textContent: exit }),
      actions);
  });
  $('jobs').replaceChildren(...rows);
  $('empty').hidden = rows.length > 0;
  $('empty').textContent = jobs.length ? 'No jobs match this filter.' : 'No jobs yet. Run one above.';
}

async function refresh() {
  try {
    jobs = await (await api('/api/jobs')).json();
    renderStats();
    renderJobs();
  } catch (err) {
    console.warn('refresh failed:', err.message);
  }
}

async function stopJob(job) {
  try {
    await api(`/api/jobs/${encodeURIComponent(job.id)}/stop`, { method: 'POST' });
  } catch (err) {
    alert('Stop failed: ' + err.message);
  }
  refresh();
}

async function deleteJob(job) {
  if (!confirm(`Delete job ${job.id} and its log?`)) return;
  try {
    await api(`/api/jobs/${encodeURIComponent(job.id)}`, { method: 'DELETE' });
  } catch (err) {
    alert('Delete failed: ' + err.message);
  }
  refresh();
}

async function openLog(job) {
  if (logAbort) logAbort.abort();
  logAbort = new AbortController();
  const out = $('logOutput');
  out.textContent = '';
  $('logTitle').textContent = `${job.name} · ${job.id}`;
  $('logCmd').textContent = job.cmd;
  $('logState').textContent = 'Streaming…';
  $('logDialog').showModal();

  try {
    const res = await api(`/api/jobs/${encodeURIComponent(job.id)}/log?follow=1`, { signal: logAbort.signal });
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      const atBottom = out.scrollTop + out.clientHeight >= out.scrollHeight - 4;
      out.textContent += decoder.decode(value, { stream: true });
      if (atBottom) out.scrollTop = out.scrollHeight;
    }
    $('logState').textContent = 'Job finished.';
    refresh();
  } catch (err) {
    if (err.name !== 'AbortError') $('logState').textContent = 'Error: ' + err.message;
  }
}

function closeLog() {
  if (logAbort) logAbort.abort();
  logAbort = null;
  $('logDialog').close();
}

window.addEventListener('DOMContentLoaded', () => {
  $('runForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    $('runError').hidden = true;
    try {
      await api('/api/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: $('nameInput').value.trim(), cmd: $('cmdInput').value }),
      });
      $('cmdInput').value = '';
      $('nameInput').value = '';
      refresh();
    } catch (err) {
      $('runError').textContent = err.message;
      $('runError').hidden = false;
    }
  });

  $('tokenForm').addEventListener('submit', (e) => {
    e.preventDefault();
    try { sessionStorage.setItem('jobworker-token', $('tokenInput').value); } catch { /* storage blocked */ }
    $('tokenPanel').hidden = true;
    refresh();
  });

  $('filters').addEventListener('click', (e) => {
    if (!e.target.dataset.filter) return;
    for (const b of $('filters').children) b.classList.toggle('active', b === e.target);
    filter = e.target.dataset.filter;
    renderJobs();
  });
  $('search').addEventListener('input', renderJobs);
  $('logClose').addEventListener('click', closeLog);
  $('logDialog').addEventListener('close', () => { if (logAbort) logAbort.abort(); });

  refresh();
  setInterval(refresh, 2000);
});
