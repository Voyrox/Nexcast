    const MAX_POINTS = (24 * 7) + 1;
    const DEFAULT_REFRESH_SECONDS = 20;

    function createEmptyHistory() {
      return {
        labels: [],
        instancesActual: [],
        instancesPredicted: [],
        cpu: [],
        memory: []
      };
    }

    const state = {
      config: {
        apiBase: "",
        refreshSeconds: DEFAULT_REFRESH_SECONDS
      },
      services: [],
      nodes: [],
      history: createEmptyHistory(),
      timer: null,
      lastUpdated: null,
      lastError: "",
      search: ""
    };

    const elements = {
      apiInput: document.getElementById("apiInput"),
      refreshInput: document.getElementById("refreshInput"),
      searchInput: document.getElementById("searchInput"),
      connectionForm: document.getElementById("connectionForm"),
      connectionStatus: document.getElementById("connectionStatus"),
      leaderStatus: document.getElementById("leaderStatus"),
      updatedStatus: document.getElementById("updatedStatus"),
      clusterSubtitle: document.getElementById("clusterSubtitle"),
      clusterMeta: document.getElementById("clusterMeta"),
      tableBody: document.getElementById("tableBody"),
      emptyState: document.getElementById("emptyState")
    };

    function escapeHTML(value) {
      return String(value)
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#39;");
    }

    function normalizeBaseURL(raw) {
      const value = String(raw || "").trim();
      if (!value) return "";
      const base = value.startsWith("http://") || value.startsWith("https://") ? value : `http://${value}`;
      return base.replace(/\/$/, "");
    }

    function toAPIURL(apiBase, path) {
      return `${normalizeBaseURL(apiBase)}${path}`;
    }

    function getDefaultAPIBase() {
      if (location.protocol.startsWith("http") && location.hostname) {
        return `${location.hostname}:8081`;
      }
      return "http://127.0.0.1:8081";
    }

    function loadConfig() {
      const params = new URLSearchParams(location.search);
      // Full break: legacy peer/token config is not supported.
      localStorage.removeItem("nexcast.peers");
      localStorage.removeItem("nexcast.token");

      const storedAPIBase = localStorage.getItem("nexcast.apiBase") || getDefaultAPIBase();
      const storedRefresh = Number(localStorage.getItem("nexcast.refresh") || DEFAULT_REFRESH_SECONDS);

      const apiBase = normalizeBaseURL(params.get("api") || storedAPIBase);
      const refreshSeconds = Math.max(5, Number(params.get("refresh") || storedRefresh || DEFAULT_REFRESH_SECONDS));

      state.config = { apiBase, refreshSeconds };
      elements.apiInput.value = apiBase;
      elements.refreshInput.value = String(refreshSeconds);
    }

    function persistConfig() {
      localStorage.setItem("nexcast.apiBase", state.config.apiBase);
      localStorage.setItem("nexcast.refresh", String(state.config.refreshSeconds));
    }

    function formatNumber(value, digits = 1) {
      if (!Number.isFinite(value)) return "0";
      return Number(value).toFixed(digits);
    }

    function formatAgo(date) {
      if (!date) return "-";
      return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
    }

    function getHealth(entry) {
      if (!entry.onlineNodes) {
        return { label: "Offline", className: "risk", color: "var(--red)" };
      }
      if (!entry.metricsReady || entry.offlineNodes > 0) {
        return { label: "Partial", className: "warn", color: "var(--amber)" };
      }
      return { label: "Healthy", className: "ok", color: "var(--green)" };
    }

    function getGlowColor(healthClass) {
      if (healthClass === "risk") return "rgba(255, 115, 115, 0.55)";
      if (healthClass === "warn") return "rgba(225, 181, 97, 0.55)";
      return "rgba(63, 224, 131, 0.6)";
    }

    function normalizeNode(nodeInfo, servicesState, address, error) {
      if (error) {
        return {
          address,
          online: false,
          error: error.message || String(error),
          services: [],
          nodeInfo: null
        };
      }

      return {
        address,
        online: true,
        error: "",
        nodeInfo,
        services: Array.isArray(servicesState.services) ? servicesState.services : []
      };
    }

    async function fetchNode(apiBase) {
      const [nodeInfoResponse, servicesResponse] = await Promise.all([
        fetch(toAPIURL(apiBase, "/nodeInfo")),
        fetch(toAPIURL(apiBase, "/servicesState"))
      ]);

      if (!nodeInfoResponse.ok) {
        throw new Error(`/nodeInfo ${nodeInfoResponse.status}`);
      }
      if (!servicesResponse.ok) {
        throw new Error(`/servicesState ${servicesResponse.status}`);
      }

      const [nodeInfo, servicesState] = await Promise.all([
        nodeInfoResponse.json(),
        servicesResponse.json()
      ]);

      return normalizeNode(nodeInfo, servicesState, normalizeBaseURL(apiBase));
    }

    async function fetchHistory(apiBase) {
      const response = await fetch(toAPIURL(apiBase, "/history"));
      if (!response.ok) {
        throw new Error(`/history ${response.status}`);
      }
      return await response.json();
    }

    function aggregateServices(nodes) {
      const serviceMap = new Map();

      nodes.forEach((node) => {
        if (!node.online) {
          return;
        }

        node.services.forEach((service) => {
          const key = service.serviceName || `service-${service.systemId}`;
          if (!serviceMap.has(key)) {
            serviceMap.set(key, {
              name: key,
              systemId: service.systemId,
              replicas: 0,
              rps: 0,
              cpuTotal: 0,
              memTotal: 0,
              cpuSamples: 0,
              memSamples: 0,
              metricsReady: true,
              onlineNodes: 0,
              offlineNodes: nodes.filter((item) => !item.online).length,
              contributingNodes: []
            });
          }

          const entry = serviceMap.get(key);
          entry.replicas += Number(service.currentReplicas || 0);
          entry.rps += Number(service.rps || 0);
          entry.onlineNodes += 1;
          entry.contributingNodes.push(node.address);

          if (service.metricsReady) {
            entry.cpuTotal += Number(service.avgCPU || 0);
            entry.memTotal += Number(service.avgMem || 0);
            entry.cpuSamples += 1;
            entry.memSamples += 1;
          } else {
            entry.metricsReady = false;
          }
        });
      });

      return Array.from(serviceMap.values())
        .map((entry) => ({
          ...entry,
          avgCPU: entry.cpuSamples ? entry.cpuTotal / entry.cpuSamples : 0,
          avgMem: entry.memSamples ? entry.memTotal / entry.memSamples : 0,
          metricsReady: entry.metricsReady && entry.cpuSamples > 0 && entry.memSamples > 0
        }))
        .sort((a, b) => b.replicas - a.replicas || b.rps - a.rps || a.name.localeCompare(b.name));
    }

    function computePredictedReplicas(services) {
      return services.reduce((total, service) => {
        const recentDemand = Math.max(service.replicas, Math.ceil(service.rps / 65) || 0);
        return total + recentDemand;
      }, 0);
    }

    function formatHistoryLabel(date) {
      return date.toLocaleString([], {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit"
      });
    }

    function createLiveSnapshot(services) {
      const totalReplicas = services.reduce((sum, service) => sum + service.replicas, 0);
      const avgCPU = services.length ? services.reduce((sum, service) => sum + service.avgCPU, 0) / services.length : 0;
      const avgMem = services.length ? services.reduce((sum, service) => sum + service.avgMem, 0) / services.length : 0;

      return {
        timestamp: new Date().toISOString(),
        totalReplicas,
        recommendedReplicas: computePredictedReplicas(services),
        avgCPU,
        avgMem
      };
    }

    function appendHistorySnapshot(history, snapshot) {
      if (!snapshot || !snapshot.timestamp) {
        return;
      }

      const timestamp = new Date(snapshot.timestamp);
      if (Number.isNaN(timestamp.getTime())) {
        return;
      }

      pushHistoryPoint(history.labels, formatHistoryLabel(timestamp));
      pushHistoryPoint(history.instancesActual, Number(snapshot.totalReplicas || 0));
      pushHistoryPoint(history.instancesPredicted, Number(snapshot.recommendedReplicas || 0));
      pushHistoryPoint(history.cpu, Number(snapshot.avgCPU || 0));
      pushHistoryPoint(history.memory, Number(snapshot.avgMem || 0));
    }

    function setHistoryFromBackend(historyResponse, services) {
      const nextHistory = createEmptyHistory();
      const snapshots = Array.isArray(historyResponse && historyResponse.days)
        ? historyResponse.days.flatMap((day) => Array.isArray(day.snapshots) ? day.snapshots : [])
        : [];

      snapshots
        .slice()
        .sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp))
        .forEach((snapshot) => appendHistorySnapshot(nextHistory, snapshot));

      if (services.length) {
        appendHistorySnapshot(nextHistory, createLiveSnapshot(services));
      }

      state.history = nextHistory;
    }

    function updateHistory(services) {
      if (!services.length) {
        return;
      }

      const totalReplicas = services.reduce((sum, service) => sum + service.replicas, 0);
      const avgCPU = services.length ? services.reduce((sum, service) => sum + service.avgCPU, 0) / services.length : 0;
      const avgMem = services.length ? services.reduce((sum, service) => sum + service.avgMem, 0) / services.length : 0;
      const predicted = computePredictedReplicas(services);
      const label = formatHistoryLabel(new Date());

      pushHistoryPoint(state.history.labels, label);
      pushHistoryPoint(state.history.instancesActual, totalReplicas);
      pushHistoryPoint(state.history.instancesPredicted, predicted);
      pushHistoryPoint(state.history.cpu, avgCPU);
      pushHistoryPoint(state.history.memory, avgMem);
    }

    function pushHistoryPoint(list, value) {
      list.push(value);
      if (list.length > MAX_POINTS) {
        list.shift();
      }
    }

    function renderTable() {
      const filter = state.search.trim().toLowerCase();
      const services = state.services.filter((service) => {
        if (!filter) return true;
        return service.name.toLowerCase().includes(filter) || String(service.systemId).includes(filter);
      });

      if (!services.length) {
        elements.tableBody.innerHTML = `<div class="empty-state">${escapeHTML(state.lastError || "No live services matched your search.")}</div>`;
        return;
      }

      elements.tableBody.innerHTML = services.map((service) => {
        const health = getHealth(service);
        const glow = getGlowColor(health.className);
        return `
          <div class="table-row">
            <div class="cluster-meta">
              <span class="cluster-dot" style="background: ${health.color}; box-shadow: 0 0 14px ${glow};"></span>
              <div class="cluster-name-wrap">
                <div class="cluster-name">${escapeHTML(service.name)}</div>
                <div class="cluster-id">system-${escapeHTML(service.systemId)} | ${escapeHTML(service.contributingNodes.join(", "))}</div>
              </div>
            </div>
            <div><span class="stats-pill">${escapeHTML(`${service.replicas} replicas / ${formatNumber(service.rps)} RPS / ${formatNumber(service.avgCPU)}% CPU`)}</span></div>
            <div class="status ${health.className}">${escapeHTML(health.label)}</div>
            <div class="cluster-count">${escapeHTML(String(service.onlineNodes))}</div>
          </div>`;
      }).join("");
    }

    function updateMeta() {
      const onlineNodes = state.nodes.filter((node) => node.online);
      const offlineNodes = state.nodes.filter((node) => !node.online);
      const leaders = onlineNodes.filter((node) => node.nodeInfo && node.nodeInfo.isLeader);
      const leader = leaders[0] || onlineNodes.find((node) => node.nodeInfo && node.nodeInfo.leaderAddr) || null;
      const clusterHealthy = onlineNodes.every((node) => node.nodeInfo && node.nodeInfo.clusterHealthy);

      elements.connectionStatus.textContent = state.lastError
        ? "Disconnected"
        : onlineNodes.length
          ? "Connected"
          : "Waiting for connection";
      elements.leaderStatus.textContent = leader && leader.nodeInfo ? leader.nodeInfo.leaderAddr || leader.address : "-";
      elements.updatedStatus.textContent = formatAgo(state.lastUpdated);
      elements.clusterSubtitle.textContent = onlineNodes.length
        ? `Live data from the Nexcast instance. Health is ${clusterHealthy && offlineNodes.length === 0 ? "stable" : "partial"}.`
        : "Connect to the Nexcast API to load live service state.";
      elements.clusterMeta.textContent = `${state.services.length} services loaded`;
    }

    function getChartConfig(kind) {
      if (kind === "instances") {
        return {
          canvas: document.getElementById("instancesChart"),
          labels: state.history.labels,
          series: [
            { color: "#3fe083", values: state.history.instancesActual },
            { color: "#51a8ff", values: state.history.instancesPredicted }
          ],
          yMax: Math.max(8, ...state.history.instancesActual, ...state.history.instancesPredicted) * 1.2
        };
      }

      return {
        canvas: document.getElementById("nodeChart"),
        labels: state.history.labels,
        series: [
          { color: "#f0d38a", values: state.history.cpu },
          { color: "#ff8a8a", values: state.history.memory }
        ],
        yMax: 100
      };
    }

    function drawChart(config) {
      const canvas = config.canvas;
      const rect = canvas.getBoundingClientRect();
      const width = Math.max(0, rect.width);
      const height = Math.max(220, rect.height || 220);
      const ratio = window.devicePixelRatio || 1;
      const ctx = canvas.getContext("2d");

      canvas.width = width * ratio;
      canvas.height = height * ratio;
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
      ctx.clearRect(0, 0, width, height);
      ctx.fillStyle = "#0f0f0f";
      ctx.fillRect(0, 0, width, height);

      const padding = { top: 18, right: 18, bottom: 34, left: 42 };
      const chartWidth = Math.max(1, width - padding.left - padding.right);
      const chartHeight = Math.max(1, height - padding.top - padding.bottom);
      const yTicks = [0.25, 0.5, 0.75, 1].map((step) => Math.round(config.yMax * step));

      ctx.strokeStyle = "rgba(255,255,255,0.08)";
      ctx.lineWidth = 1;
      yTicks.forEach((tick) => {
        const y = padding.top + chartHeight - (tick / config.yMax) * chartHeight;
        ctx.beginPath();
        ctx.moveTo(padding.left, y);
        ctx.lineTo(width - padding.right, y);
        ctx.stroke();
        ctx.fillStyle = "#7f7f7f";
        ctx.font = '12px "Segoe UI", sans-serif';
        ctx.fillText(String(tick), 8, y + 4);
      });

      if (!config.labels.length) {
        ctx.fillStyle = "#7f7f7f";
        ctx.font = '14px "Segoe UI", sans-serif';
        ctx.fillText("Waiting for live data", padding.left, padding.top + 20);
        return;
      }

      const tickIndexes = config.labels.length <= 6
        ? config.labels.map((_, index) => index)
        : [0, Math.floor(config.labels.length * 0.25), Math.floor(config.labels.length * 0.5), Math.floor(config.labels.length * 0.75), config.labels.length - 1];

      tickIndexes.forEach((index) => {
        const x = padding.left + (index / Math.max(1, config.labels.length - 1)) * chartWidth;
        ctx.beginPath();
        ctx.moveTo(x, padding.top);
        ctx.lineTo(x, padding.top + chartHeight);
        ctx.strokeStyle = "rgba(255,255,255,0.04)";
        ctx.stroke();
        ctx.fillStyle = "#7f7f7f";
        ctx.font = '12px "Segoe UI", sans-serif';
        ctx.fillText(config.labels[index], x - 16, height - 10);
      });

      config.series.forEach((series) => {
        if (!series.values.length) return;
        const points = series.values.map((value, index) => ({
          x: padding.left + (index / Math.max(1, series.values.length - 1)) * chartWidth,
          y: padding.top + chartHeight - (Math.max(0, value) / config.yMax) * chartHeight
        }));

        const gradient = ctx.createLinearGradient(0, padding.top, 0, padding.top + chartHeight);
        gradient.addColorStop(0, `${series.color}44`);
        gradient.addColorStop(1, `${series.color}00`);

        ctx.beginPath();
        points.forEach((point, index) => {
          if (index === 0) ctx.moveTo(point.x, point.y);
          else ctx.lineTo(point.x, point.y);
        });
        ctx.lineTo(points[points.length - 1].x, padding.top + chartHeight);
        ctx.lineTo(points[0].x, padding.top + chartHeight);
        ctx.closePath();
        ctx.fillStyle = gradient;
        ctx.fill();

        ctx.beginPath();
        points.forEach((point, index) => {
          if (index === 0) ctx.moveTo(point.x, point.y);
          else ctx.lineTo(point.x, point.y);
        });
        ctx.strokeStyle = series.color;
        ctx.lineWidth = 2.2;
        ctx.stroke();

        points.forEach((point) => {
          ctx.beginPath();
          ctx.arc(point.x, point.y, 2.4, 0, Math.PI * 2);
          ctx.fillStyle = series.color;
          ctx.fill();
        });
      });
    }

    function renderCharts() {
      drawChart(getChartConfig("instances"));
      drawChart(getChartConfig("nodes"));
    }

    async function refreshData() {
      if (!state.config.apiBase) {
        state.lastError = "Missing API URL.";
        state.services = [];
        state.nodes = [];
        updateMeta();
        renderTable();
        renderCharts();
        return;
      }

      let node;
      try {
        node = await fetchNode(state.config.apiBase);
      } catch (error) {
        node = normalizeNode(null, null, normalizeBaseURL(state.config.apiBase), error || new Error("request failed"));
      }

      state.nodes = [node];
      state.lastError = node.online ? "" : `${node.address} ${node.error}`;
      state.services = node.online ? aggregateServices(state.nodes) : [];
      state.lastUpdated = new Date();

      if (node.online && state.services.length) {
        try {
          const historyResponse = await fetchHistory(state.config.apiBase);
          setHistoryFromBackend(historyResponse, state.services);
        } catch (error) {
          updateHistory(state.services);
        }
      } else {
        state.history = createEmptyHistory();
      }

      updateMeta();
      renderTable();
      renderCharts();
    }

    function startPolling() {
      if (state.timer) {
        clearInterval(state.timer);
      }

      refreshData();
      state.timer = window.setInterval(refreshData, state.config.refreshSeconds * 1000);
    }

    elements.connectionForm.addEventListener("submit", (event) => {
      event.preventDefault();
      state.config.apiBase = normalizeBaseURL(elements.apiInput.value);
      state.config.refreshSeconds = Math.max(5, Number(elements.refreshInput.value || DEFAULT_REFRESH_SECONDS));
      state.history = createEmptyHistory();
      persistConfig();
      startPolling();
    });

    elements.searchInput.addEventListener("input", (event) => {
      state.search = event.target.value || "";
      renderTable();
    });

    window.addEventListener("resize", renderCharts);

    loadConfig();
    updateMeta();
    renderTable();
    renderCharts();
    if (state.config.apiBase) {
      startPolling();
    }
