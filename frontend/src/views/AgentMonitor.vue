<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import {
  ArrowRight,
  Bot,
  Brain,
  CheckCircle,
  Clock,
  Cpu,
  GitBranch,
  RefreshCcw,
  Shield,
  Users,
  Workflow,
  XCircle,
  Zap,
} from "lucide-vue-next";
import { APIInfo } from "../api/wails";

// ── 类型定义 ──
interface AgentInfoItem {
  type: string;
  name: string;
  status: string;
  tasks_handled: number;
  error_count: number;
  last_active: string;
}
interface WfStep {
  name: string;
  agent: string;
  task_type: string;
  next: string[];
  on_error: string;
}
interface WfDef {
  name: string;
  type: string;
  description: string;
  steps: WfStep[];
  start_step: string;
}
interface SystemStateData {
  agents: AgentInfoItem[];
  rate_limit: Record<string, any>;
  active_workflow: Record<string, any> | null;
  workflows: WfDef[];
  last_update: string;
}

// ── 状态数据 ──
const systemState = ref<SystemStateData | null>(null);
const loading = ref(false);
const autoRefresh = ref(true);
const selectedWorkflow = ref<string>("request_process");
let refreshTimer: ReturnType<typeof setInterval> | null = null;

// ── 获取系统状态（调用真实后端 API） ──
const fetchSystemState = async () => {
  try {
    const data = await (APIInfo as any).getAgentSystemState();
    if (data) {
      systemState.value = data;
    }
  } catch (error) {
    console.error("获取 Agent 系统状态失败:", error);
  }
};

const refreshState = async () => {
  loading.value = true;
  await fetchSystemState();
  loading.value = false;
};

// ── Agent 相关计算属性 ──
const agentStates = computed(() => {
  if (!systemState.value?.agents) return [];
  return systemState.value.agents.map((a) => ({
    ...a,
    icon: getAgentIcon(a.type),
    color: getAgentColor(a.status),
    bgColor: getAgentBgColor(a.type),
  }));
});

const getAgentIcon = (type: string) => {
  const map: Record<string, any> = {
    orchestrator: Brain,
    rate_limiter: Shield,
    account_manager: Users,
    model_selector: Cpu,
    request_router: GitBranch,
  };
  return map[type] || Bot;
};

const getAgentColor = (status: string) => {
  const map: Record<string, string> = {
    running: "text-emerald-500",
    idle: "text-blue-500",
    waiting: "text-amber-500",
    error: "text-rose-500",
    stopped: "text-slate-400",
  };
  return map[status] || "text-slate-500";
};

const getAgentBgColor = (type: string) => {
  const map: Record<string, string> = {
    rate_limiter: "from-rose-500/8 to-rose-500/3",
    account_manager: "from-blue-500/8 to-blue-500/3",
    model_selector: "from-amber-500/8 to-amber-500/3",
    request_router: "from-purple-500/8 to-purple-500/3",
  };
  return map[type] || "from-slate-500/8 to-slate-500/3";
};

const getAgentName = (type: string) => {
  const map: Record<string, string> = {
    orchestrator: "主控编排器",
    rate_limiter: "限速监控",
    account_manager: "账号管理",
    model_selector: "模型选择",
    request_router: "请求路由",
  };
  return map[type] || type;
};

const getStatusLabel = (status: string) => {
  const map: Record<string, string> = {
    running: "运行中",
    idle: "空闲",
    waiting: "等待中",
    error: "错误",
    stopped: "已停止",
  };
  return map[status] || status;
};

// ── 限速状态 ──
const rateLimitState = computed(() => systemState.value?.rate_limit);
const isRateLimited = computed(() => rateLimitState.value?.is_limited || false);

// ── 活跃工作流 ──
const activeWorkflow = computed(() => systemState.value?.active_workflow);

// ── 工作流定义列表 ──
const workflows = computed<WfDef[]>(() => systemState.value?.workflows || []);

// ── 当前选中的工作流定义 ──
const currentWorkflow = computed(() =>
  workflows.value.find((w) => w.type === selectedWorkflow.value) || workflows.value[0] || null,
);

// ── 工作流步骤链（用于 DAG 可视化） ──
const workflowChain = computed(() => {
  const wf = currentWorkflow.value;
  if (!wf) return [];
  // 从 start_step 开始，沿主路径（next[0]）构建有序链
  const stepMap = new Map<string, WfStep>();
  wf.steps.forEach((s) => stepMap.set(s.name, s));
  const chain: WfStep[] = [];
  const visited = new Set<string>();
  let current = wf.start_step;
  while (current && !visited.has(current)) {
    visited.add(current);
    const step = stepMap.get(current);
    if (!step) break;
    chain.push(step);
    current = step.next?.[0] || "";
  }
  return chain;
});

// ── 错误分支步骤（非主路径的步骤） ──
const errorBranchSteps = computed(() => {
  const wf = currentWorkflow.value;
  if (!wf) return [];
  const mainNames = new Set(workflowChain.value.map((s) => s.name));
  return wf.steps.filter((s) => !mainNames.has(s.name));
});

// ── Agent 任务分配统计 ──
const agentTaskDistribution = computed(() => {
  const wf = currentWorkflow.value;
  if (!wf) return [];
  const dist: Record<string, { agent: string; name: string; steps: string[]; color: string }> = {};
  wf.steps.forEach((s) => {
    if (!dist[s.agent]) {
      dist[s.agent] = {
        agent: s.agent,
        name: getAgentName(s.agent),
        steps: [],
        color: getAgentDistColor(s.agent),
      };
    }
    dist[s.agent].steps.push(s.name);
  });
  return Object.values(dist);
});

const getAgentDistColor = (type: string) => {
  const map: Record<string, string> = {
    rate_limiter: "bg-rose-500",
    account_manager: "bg-blue-500",
    model_selector: "bg-amber-500",
    request_router: "bg-purple-500",
  };
  return map[type] || "bg-slate-500";
};

const getStepAgentColor = (agentType: string) => {
  const map: Record<string, string> = {
    rate_limiter: "border-rose-400/40 bg-rose-500/6",
    account_manager: "border-blue-400/40 bg-blue-500/6",
    model_selector: "border-amber-400/40 bg-amber-500/6",
    request_router: "border-purple-400/40 bg-purple-500/6",
  };
  return map[agentType] || "border-slate-400/40 bg-slate-500/6";
};

// ── 判断步骤是否为活跃工作流的当前步骤 ──
const isActiveStep = (stepName: string) => {
  const aw = activeWorkflow.value;
  if (!aw || aw.status !== "running") return false;
  return aw.current_step === stepName;
};

// ── 自动刷新 ──
const startAutoRefresh = () => {
  refreshTimer = setInterval(fetchSystemState, 5000);
};
const stopAutoRefresh = () => {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
};
const toggleAutoRefresh = () => {
  autoRefresh.value = !autoRefresh.value;
  autoRefresh.value ? startAutoRefresh() : stopAutoRefresh();
};

onMounted(() => {
  fetchSystemState();
  if (autoRefresh.value) startAutoRefresh();
});
onUnmounted(() => {
  stopAutoRefresh();
});
</script>

<template>
  <div class="space-y-6 p-6">
    <!-- ==================== 标题栏 ==================== -->
    <section
      class="ios-glass overflow-hidden rounded-[28px] border border-black/[0.05] shadow-[0_20px_48px_-20px_rgba(15,23,42,0.18)] dark:border-white/[0.06]"
    >
      <div
        class="border-b border-black/[0.05] bg-[radial-gradient(circle_at_top_left,rgba(168,85,247,0.16),transparent_35%),linear-gradient(180deg,rgba(255,255,255,0.82),rgba(255,255,255,0.68))] px-6 py-5 dark:border-white/[0.06] dark:bg-[radial-gradient(circle_at_top_left,rgba(168,85,247,0.18),transparent_35%),linear-gradient(180deg,rgba(28,28,30,0.94),rgba(28,28,30,0.84))]"
      >
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="flex min-w-0 items-start gap-3">
            <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-purple-500/10 text-purple-600 shadow-inner">
              <Brain class="h-5 w-5" stroke-width="2.4" />
            </div>
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h1 class="text-[17px] font-bold text-ios-text dark:text-ios-textDark">AI 工作流编排</h1>
                <span class="rounded-full bg-purple-500/10 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-purple-600">Multi-Agent</span>
              </div>
              <p class="mt-1 max-w-3xl text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                多智能体协作工作流 - 实时可视化 Agent 协作、任务分配、工作流 DAG 与执行状态
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button type="button" class="no-drag-region inline-flex items-center gap-2 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2 text-[12px] font-semibold text-ios-text shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textDark" :class="autoRefresh ? 'border-emerald-500/30 text-emerald-600' : ''" @click="toggleAutoRefresh">
              <RefreshCcw class="h-3.5 w-3.5" :class="loading ? 'animate-spin' : ''" stroke-width="2.4" />
              {{ autoRefresh ? "自动刷新中" : "开启自动刷新" }}
            </button>
            <button type="button" class="no-drag-region inline-flex items-center gap-2 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2 text-[12px] font-semibold text-ios-text shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textDark" :disabled="loading" @click="refreshState">
              <RefreshCcw class="h-3.5 w-3.5" :class="loading ? 'animate-spin' : ''" stroke-width="2.4" />
              刷新
            </button>
          </div>
        </div>
      </div>

      <!-- Agent 状态卡片 -->
      <div class="grid grid-cols-1 gap-3 p-6 md:grid-cols-2 xl:grid-cols-4">
        <article
          v-for="agent in agentStates"
          :key="agent.type"
          class="rounded-[22px] border border-black/[0.05] bg-gradient-to-b p-4 shadow-sm dark:border-white/[0.06]"
          :class="agent.bgColor"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <div class="text-[11px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                {{ agent.name }}
              </div>
              <div class="mt-2 text-[20px] font-extrabold" :class="agent.color">
                {{ getStatusLabel(agent.status) }}
              </div>
            </div>
            <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-black/[0.03] dark:bg-white/[0.06]">
              <component :is="agent.icon" class="h-5 w-5" :class="agent.color" stroke-width="2.4" />
            </div>
          </div>
          <div class="mt-3 flex items-center justify-between text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
            <span>已处理 {{ agent.tasks_handled }} 个任务</span>
            <span v-if="agent.error_count > 0" class="text-rose-500 font-semibold">{{ agent.error_count }} 错误</span>
          </div>
        </article>
      </div>
    </section>

    <!-- ==================== 工作流 DAG 可视化 ==================== -->
    <section class="ios-glass rounded-[24px] border border-black/[0.05] p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)] dark:border-white/[0.06]">
      <div class="flex items-center justify-between gap-4">
        <div class="flex items-center gap-2">
          <div class="flex h-9 w-9 items-center justify-center rounded-2xl bg-indigo-500/10 text-indigo-600">
            <Workflow class="h-4 w-4" stroke-width="2.4" />
          </div>
          <div>
            <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">工作流 DAG</div>
            <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">任务编排流程可视化</div>
          </div>
        </div>
        <!-- 工作流切换 -->
        <div class="flex items-center gap-2">
          <button
            v-for="wf in workflows"
            :key="wf.type"
            type="button"
            class="no-drag-region rounded-full px-3 py-1.5 text-[11px] font-semibold transition-all"
            :class="selectedWorkflow === wf.type ? 'bg-indigo-500 text-white shadow-md shadow-indigo-500/25' : 'bg-black/[0.04] text-ios-textSecondary hover:bg-black/[0.08] dark:bg-white/[0.06] dark:text-ios-textSecondaryDark'"
            @click="selectedWorkflow = wf.type"
          >
            {{ wf.name }}
          </button>
        </div>
      </div>

      <!-- 工作流描述 -->
      <div v-if="currentWorkflow" class="mt-3 rounded-[14px] bg-black/[0.02] px-4 py-2 dark:bg-white/[0.03]">
        <p class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">{{ currentWorkflow.description }}</p>
      </div>

      <!-- DAG 主路径 -->
      <div v-if="workflowChain.length" class="mt-5 overflow-x-auto">
        <div class="flex items-center gap-1 min-w-max py-2 px-1">
          <template v-for="(step, idx) in workflowChain" :key="step.name">
            <!-- 步骤节点 -->
            <div
              class="relative flex flex-col items-center gap-1.5 rounded-[16px] border-2 px-4 py-3 transition-all min-w-[120px]"
              :class="[
                getStepAgentColor(step.agent),
                isActiveStep(step.name) ? 'ring-2 ring-indigo-500 ring-offset-2 dark:ring-offset-[#1c1c1e] shadow-lg scale-105' : '',
              ]"
            >
              <!-- 活跃指示器 -->
              <div v-if="isActiveStep(step.name)" class="absolute -top-1.5 -right-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-indigo-500 shadow">
                <div class="h-2 w-2 animate-pulse rounded-full bg-white"></div>
              </div>
              <component :is="getAgentIcon(step.agent)" class="h-4 w-4" :class="getAgentColor('running')" stroke-width="2" />
              <div class="text-[11px] font-bold text-ios-text dark:text-ios-textDark text-center whitespace-nowrap">
                {{ step.name.replace(/_/g, ' ') }}
              </div>
              <div class="text-[9px] font-medium text-ios-textSecondary dark:text-ios-textSecondaryDark">
                {{ getAgentName(step.agent) }}
              </div>
            </div>
            <!-- 箭头 -->
            <ArrowRight v-if="idx < workflowChain.length - 1" class="h-4 w-4 shrink-0 text-slate-400 dark:text-slate-500" stroke-width="2" />
          </template>
          <!-- 完成标记 -->
          <div class="flex flex-col items-center gap-1 rounded-[14px] border-2 border-emerald-400/40 bg-emerald-500/6 px-3 py-3 min-w-[80px]">
            <CheckCircle class="h-4 w-4 text-emerald-500" stroke-width="2" />
            <div class="text-[10px] font-bold text-emerald-600">完成</div>
          </div>
        </div>
      </div>

      <!-- 错误分支 -->
      <div v-if="errorBranchSteps.length" class="mt-4">
        <div class="mb-2 text-[11px] font-bold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark">
          异常处理分支
        </div>
        <div class="flex flex-wrap gap-2">
          <div
            v-for="step in errorBranchSteps"
            :key="step.name"
            class="flex items-center gap-2 rounded-[12px] border border-rose-400/20 bg-rose-500/5 px-3 py-2"
          >
            <XCircle class="h-3.5 w-3.5 text-rose-400" stroke-width="2" />
            <div>
              <div class="text-[11px] font-semibold text-ios-text dark:text-ios-textDark">{{ step.name.replace(/_/g, ' ') }}</div>
              <div class="text-[9px] text-ios-textSecondary">{{ getAgentName(step.agent) }} / {{ step.task_type }}</div>
            </div>
            <ArrowRight class="h-3 w-3 text-rose-300" stroke-width="2" />
            <span class="text-[9px] font-medium text-rose-500">{{ step.next?.[0]?.replace(/_/g, ' ') || '结束' }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- ==================== Agent 任务分配 ==================== -->
    <section class="grid grid-cols-1 gap-6 xl:grid-cols-2">
      <!-- 任务分配矩阵 -->
      <div class="ios-glass rounded-[24px] border border-black/[0.05] p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)] dark:border-white/[0.06]">
        <div class="flex items-center gap-2">
          <div class="flex h-9 w-9 items-center justify-center rounded-2xl bg-amber-500/10 text-amber-600">
            <Zap class="h-4 w-4" stroke-width="2.4" />
          </div>
          <div>
            <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">Agent 任务分配</div>
            <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">当前工作流中各 Agent 负责的步骤</div>
          </div>
        </div>
        <div class="mt-4 space-y-3">
          <div
            v-for="item in agentTaskDistribution"
            :key="item.agent"
            class="rounded-[14px] bg-black/[0.02] p-3 dark:bg-white/[0.03]"
          >
            <div class="flex items-center gap-2 mb-2">
              <span class="inline-block h-2.5 w-2.5 rounded-full" :class="item.color"></span>
              <span class="text-[12px] font-bold text-ios-text dark:text-ios-textDark">{{ item.name }}</span>
              <span class="ml-auto rounded-full bg-black/[0.05] px-2 py-0.5 text-[10px] font-semibold text-ios-textSecondary dark:bg-white/[0.08]">
                {{ item.steps.length }} 个步骤
              </span>
            </div>
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="stepName in item.steps"
                :key="stepName"
                class="rounded-[8px] bg-white/80 px-2 py-1 text-[10px] font-medium text-ios-text shadow-sm dark:bg-white/[0.06] dark:text-ios-textDark"
                :class="isActiveStep(stepName) ? 'ring-1 ring-indigo-500 text-indigo-600 font-bold' : ''"
              >
                {{ stepName.replace(/_/g, ' ') }}
              </span>
            </div>
          </div>
          <div v-if="!agentTaskDistribution.length" class="py-6 text-center text-[12px] text-ios-textSecondary">
            暂无工作流数据
          </div>
        </div>
      </div>

      <!-- 限速 & 活跃工作流状态 -->
      <div class="space-y-6">
        <!-- 限速状态 -->
        <div
          class="ios-glass rounded-[24px] border p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)]"
          :class="isRateLimited ? 'border-rose-500/20 dark:border-rose-500/10' : 'border-black/[0.05] dark:border-white/[0.06]'"
        >
          <div class="flex items-center gap-2">
            <div class="flex h-9 w-9 items-center justify-center rounded-2xl" :class="isRateLimited ? 'bg-rose-500/10 text-rose-600' : 'bg-emerald-500/10 text-emerald-600'">
              <Shield class="h-4 w-4" stroke-width="2.4" />
            </div>
            <div>
              <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">限速状态</div>
              <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">{{ isRateLimited ? "当前已被限速" : "正常运行中" }}</div>
            </div>
          </div>
          <div class="mt-4 space-y-2">
            <div class="flex items-center justify-between rounded-[14px] bg-black/[0.03] px-3 py-2 dark:bg-white/[0.04]">
              <span class="text-[12px] text-ios-textSecondary">状态</span>
              <span class="text-[12px] font-bold" :class="isRateLimited ? 'text-rose-600' : 'text-emerald-600'">{{ isRateLimited ? "限速中" : "正常" }}</span>
            </div>
            <div class="flex items-center justify-between rounded-[14px] bg-black/[0.03] px-3 py-2 dark:bg-white/[0.04]">
              <span class="text-[12px] text-ios-textSecondary">剩余请求数</span>
              <span class="text-[12px] font-bold text-ios-text dark:text-ios-textDark">{{ rateLimitState?.remaining_requests ?? "N/A" }}</span>
            </div>
            <div class="flex items-center justify-between rounded-[14px] bg-black/[0.03] px-3 py-2 dark:bg-white/[0.04]">
              <span class="text-[12px] text-ios-textSecondary">剩余 Token</span>
              <span class="text-[12px] font-bold text-ios-text dark:text-ios-textDark">{{ rateLimitState?.remaining_tokens?.toLocaleString() ?? "N/A" }}</span>
            </div>
          </div>
        </div>

        <!-- 活跃工作流 -->
        <div class="ios-glass rounded-[24px] border border-black/[0.05] p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)] dark:border-white/[0.06]">
          <div class="flex items-center gap-2">
            <div class="flex h-9 w-9 items-center justify-center rounded-2xl bg-blue-500/10 text-blue-600 dark:text-blue-300">
              <Workflow class="h-4 w-4" stroke-width="2.4" />
            </div>
            <div>
              <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">活跃工作流</div>
              <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">当前正在执行的工作流</div>
            </div>
          </div>
          <div v-if="activeWorkflow" class="mt-4 space-y-2">
            <div class="flex items-center justify-between rounded-[14px] bg-black/[0.03] px-3 py-2 dark:bg-white/[0.04]">
              <span class="text-[12px] text-ios-textSecondary">工作流</span>
              <span class="text-[12px] font-bold text-ios-text dark:text-ios-textDark">{{ activeWorkflow.workflow_name }}</span>
            </div>
            <div class="flex items-center justify-between rounded-[14px] bg-black/[0.03] px-3 py-2 dark:bg-white/[0.04]">
              <span class="text-[12px] text-ios-textSecondary">当前步骤</span>
              <span class="text-[12px] font-bold text-indigo-600">{{ activeWorkflow.current_step?.replace(/_/g, ' ') }}</span>
            </div>
            <div class="flex items-center justify-between rounded-[14px] bg-black/[0.03] px-3 py-2 dark:bg-white/[0.04]">
              <span class="text-[12px] text-ios-textSecondary">状态</span>
              <span class="text-[12px] font-bold" :class="activeWorkflow.status === 'running' ? 'text-emerald-600' : activeWorkflow.status === 'completed' ? 'text-blue-600' : 'text-rose-500'">
                {{ activeWorkflow.status === 'running' ? '执行中' : activeWorkflow.status === 'completed' ? '已完成' : activeWorkflow.status }}
              </span>
            </div>
            <div class="flex items-center justify-between rounded-[14px] bg-black/[0.03] px-3 py-2 dark:bg-white/[0.04]">
              <span class="text-[12px] text-ios-textSecondary">已完成步骤</span>
              <span class="text-[12px] font-bold text-ios-text dark:text-ios-textDark">{{ activeWorkflow.results_count ?? 0 }}</span>
            </div>
          </div>
          <div v-else class="mt-4 flex flex-col items-center justify-center py-6">
            <Clock class="h-8 w-8 text-slate-300" stroke-width="1.5" />
            <p class="mt-2 text-[12px] text-ios-textSecondary">暂无活跃工作流</p>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
