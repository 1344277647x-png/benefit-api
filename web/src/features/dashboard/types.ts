/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { TimeGranularity } from '@/lib/time'

// ============================================================================
// Quota & Usage Data Types
// ============================================================================

export interface QuotaDataItem {
  id?: number
  user_id?: number
  username?: string
  model_name?: string
  created_at: number
  token_used?: number
  count?: number
  quota?: number
}

export interface FlowQuotaDataItem {
  user_id?: number
  username?: string
  node_name?: string
  use_group?: string
  token_id?: number
  token_name?: string
  channel_id?: number
  channel_name?: string
  model_name?: string
  token_used?: number
  count?: number
  quota?: number
}

export type FlowMetric = 'quota' | 'tokens' | 'requests'

export type FlowOverflowMode = 'aggregate' | 'hide'

export type FlowRole = 'user' | 'admin' | 'root'

export type FlowNodeKind =
  | 'user'
  | 'node'
  | 'token'
  | 'group'
  | 'model'
  | 'channel'

export interface FlowNodeFilter {
  kind: FlowNodeKind
  id: string
}

export interface FlowLinkSelection {
  source: string
  target: string
}

export interface FlowBuildOptions {
  role?: FlowRole
  selectedUsers?: string[]
  selectedNodes?: FlowNodeFilter[]
  activeNode?: FlowNodeFilter
  activeLink?: FlowLinkSelection
  colorPalette?: readonly string[]
  visibleStages?: FlowNodeKind[]
  topNodeLimit?: number
  overflowMode?: FlowOverflowMode
  // When true, sensitive node labels (users, tokens, nodes, groups, channels)
  // are partially masked in the rendered graph while keeping node identity so
  // the Sankey shape stays intact.
  maskSensitive?: boolean
  // Resolves the label for a token whose record no longer exists (deleted).
  // Lets the caller inject a localized string such as "Deleted (123)".
  deletedTokenLabel?: (tokenId: number) => string
  otherNodeLabel?: (kind: FlowNodeKind) => string
}

export interface DashboardFlowNode {
  id: string
  label: string
  kind: FlowNodeKind
  value: number
  requests: number
  quota: number
  tokens: number
  color: string
  colorKey: string
  highlighted?: boolean
  dimmed?: boolean
}

export interface DashboardFlowLink {
  source: string
  target: string
  value: number
  requests: number
  quota: number
  tokens: number
  sourceLabel: string
  targetLabel: string
  color: string
  linkColor: string
  linkAlpha: number
  hoverColor: string
  colorKey: string
  share: number
  highlighted?: boolean
  dimmed?: boolean
}

export interface DashboardFlowGraph {
  nodes: DashboardFlowNode[]
  links: DashboardFlowLink[]
}

export interface FlowUserFilterOption {
  value: string
  label: string
  valueLabel: string
  valueRaw: number
  color: string
}

export interface FlowNodeFilterOption {
  kind: FlowNodeKind
  value: string
  label: string
  valueLabel: string
  valueRaw: number
  color: string
}

export interface FlowFilterOptions {
  users: FlowUserFilterOption[]
  nodes: FlowNodeFilterOption[]
}

export interface FlowSummary {
  quota: number
  tokens: number
  requests: number
}

export interface ProcessedFlowData {
  summary: FlowSummary
  flow: DashboardFlowGraph
  filterOptions: FlowFilterOptions
}

// ============================================================================
// Uptime Monitoring Types
// ============================================================================

export interface UptimeMonitor {
  name: string
  uptime: number
  status: number
  group?: string
}

export interface UptimeGroupResult {
  categoryName: string
  monitors: UptimeMonitor[]
}

// ============================================================================
// Dashboard Filter Types
// ============================================================================

export interface DashboardFilters {
  start_timestamp?: Date
  end_timestamp?: Date
  time_granularity?: TimeGranularity
  username?: string
}

export type ConsumptionDistributionChartType = 'bar' | 'area'

export type ModelAnalyticsChartTab = 'trend' | 'proportion' | 'top'

export interface DashboardChartPreferences {
  consumptionDistributionChart: ConsumptionDistributionChartType
  modelAnalyticsChart: ModelAnalyticsChartTab
  defaultTimeRangeDays: number
  defaultTimeGranularity: TimeGranularity
}

// User analytics selections are held by the dashboard parent so they survive
// switching between dashboard sub-sections, matching the model/flow filters.
export interface UserChartsFilters {
  timeGranularity: TimeGranularity
  selectedRange: number
  topUserLimit: number
}

// ============================================================================
// API Info Types
// ============================================================================

export interface ApiInfoItem {
  url: string
  route: string
  description: string
  color: string
}

export interface PingStatus {
  latency: number | null
  testing: boolean
  error: boolean
}

export type PingStatusMap = Record<string, PingStatus>

// ============================================================================
// Chart Types
// ============================================================================

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type VChartSpec = Record<string, any>

export interface ProcessedChartData {
  spec_pie: VChartSpec
  spec_line: VChartSpec
  spec_area: VChartSpec
  spec_model_line: VChartSpec
  spec_rank_bar: VChartSpec
  totalQuotaDisplay: string
  totalCountDisplay: string
}

export interface ProcessedUserChartData {
  spec_user_rank: VChartSpec
  spec_user_trend: VChartSpec
}

// ============================================================================
// Announcement Types
// ============================================================================

export interface AnnouncementItem {
  id?: number
  content: string
  publishDate?: string
  type?: 'default' | 'ongoing' | 'success' | 'warning' | 'error'
  extra?: string
  popupEnabled?: boolean
  popupFrequency?: 'once' | 'daily' | 'session'
}

// ============================================================================
// FAQ Types
// ============================================================================

export interface FAQItem {
  id?: number
  question: string
  answer: string
}

// ============================================================================
// Administrator operations report types
// ============================================================================

export type OperationsReportGroupBy =
  | 'day'
  | 'model'
  | 'channel'
  | 'user'
  | 'group'

export interface OperationsReportRow {
  matched_requests?: number
  pending_requests?: number
  matched_quota?: number
  partial_profit_usd?: number | null
  errors_without_quota?: number
  reasons?: Record<string, number> | null
  pending_samples?:
    | {
        request_id: string
        log_id?: number
        channel_id: number
        created_at: number
        reasons: string[]
      }[]
    | null
  unknown_revenue_requests?: number
  revenue_status?: 'known' | 'partial' | 'unknown'
  key: string
  label: string
  model_name?: string
  channel_id?: number
  channel_name?: string
  user_id?: number
  username?: string
  group?: string
  request_count: number
  success_count: number
  error_count: number
  error_rate: number
  token_count: number
  revenue_quota: number
  refund_quota: number
  revenue_usd: number
  refund_usd: number
  cost_usd: number
  cost_status: 'known' | 'partial' | 'unknown'
  unknown_cost_requests: number
  profit_usd?: number
}

export type OperationsReportSummary = Omit<OperationsReportRow, 'key' | 'label'>

export interface OperationsReportChannel {
  channel_id: number
  name: string
  balance: number
  used_quota: number
  status: number
  response_time: number
}

export interface OperationsReportHealth {
  channel_id: number
  model: string
  status: 'normal' | 'delayed' | 'unavailable' | 'unknown'
  request_count: number
  success_rate: number
  average_latency_ms: number
  average_ttft_ms: number
}

export interface OperationsReport {
  after_cost_effective?: boolean
  excluded_requests?: number
  accounting_started_at?: number
  funding?: {
    receipts_known_usd: number
    expiry_known_usd: number
    unknown_entries: number
    truncated: boolean
  }
  generated_at: number
  start_timestamp: number
  end_timestamp: number
  group_by: OperationsReportGroupBy
  truncated: boolean
  pre_aggregated?: boolean
  rollup_hours_used?: number
  quota_per_unit: number
  summary: OperationsReportSummary
  rows: OperationsReportRow[]
  channel_ids: number[]
  channels: OperationsReportChannel[]
  health: OperationsReportHealth[]
  warnings?: string[]
}

// ============================================================================
// Administrator operations health alerts & read-only billing reconciliation
// ============================================================================

export type OpsAlertSeverity = 'ok' | 'warning' | 'critical'

export interface OpsAlertTLSCertStatus {
  status: 'ok' | 'warning' | 'critical' | 'not_configured' | 'error'
  domain: string
  issuer?: string
  not_after?: number
  days_remaining: number
  warning_threshold_days: number
  checked_at: number
  error?: string
}

export interface OpsQueueSummary {
  operations_pending: number
  operations_retrying: number
  operations_oldest_pending_at: number
  business_pending: number
  business_retrying: number
  business_dead_letter: number
  billing_exceptions_count: number
  unsettled_pre_consume_count: number
}

export interface OpsAlertItem {
  code: string
  severity: 'warning' | 'critical'
  title: string
  detail: string
  count?: number
  action_path?: string
}

export interface OpsAlertsSnapshot {
  generated_at: number
  alert_level: OpsAlertSeverity
  tls: OpsAlertTLSCertStatus
  queues: OpsQueueSummary
  alerts: OpsAlertItem[]
}

export type BillingReconciliationKind =
  | 'all'
  | 'settlement_failed'
  | 'partial_settlement_failed'
  | 'accounting_projection_failed'
  | 'unsettled_pre_consume'

export interface BillingReconciliationItem {
  source: 'business_event' | 'subscription_pre_consume'
  kind: Exclude<BillingReconciliationKind, 'all'>
  event_key?: string
  request_id: string
  user_id: number
  username?: string
  subscription_id?: number
  funding_source?: string
  model_name?: string
  failure_stage?: string
  status: string
  pre_consumed_quota: number
  actual_quota?: number
  diff_quota?: number
  token_diff_quota?: number
  projected_quota?: number
  pre_consumed_usd: number
  actual_usd?: number
  diff_usd?: number
  has_consume_log: boolean
  delivered_at?: number
  dead_letter_at?: number
  created_at: number
  summary: string
}

export interface BillingReconciliationSummary {
  total_items: number
  settlement_exception_count: number
  projection_failure_count: number
  unsettled_pre_consume_count: number
  unknown_actual_count: number
  known_pre_consumed_quota: number
  known_actual_quota: number
  known_net_diff_quota: number
  unknown_pre_consumed_quota: number
  known_pre_consumed_usd: number
  known_actual_usd: number
  known_net_diff_usd: number
  unknown_pre_consumed_usd: number
}

export interface BillingReconciliationReport {
  read_only: boolean
  generated_at: number
  start_timestamp: number
  end_timestamp: number
  kind: BillingReconciliationKind
  quota_per_unit: number
  truncated: boolean
  summary: BillingReconciliationSummary
  items: BillingReconciliationItem[]
}
