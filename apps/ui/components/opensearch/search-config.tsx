import dynamic from 'next/dynamic';

// 动态导入EUI组件避免SSR问题
const EuiHealth = dynamic(
  () => import('@elastic/eui').then(mod => ({ default: mod.EuiHealth })),
  { ssr: false }
);

// EUI SearchBar Schema配置 - 适配Elasticsearch Query String语法
export const searchSchema = {
  strict: false, // 设置为false以支持更灵活的查询语法
  fields: {
    'alert.severity': {
      type: 'string',
    },
    'alert.risk_score': {
      type: 'number',
    },
    'alert.evidence.event_type': {
      type: 'string',
    },
    'metadata.host': {
      type: 'string',
    },
    'alert.evidence.process_name': {
      type: 'string',
    },
    'metadata.source': {
      type: 'string',
    },
    '@timestamp': {
      type: 'date',
    },
  },
};

// EUI SearchBar 过滤器配置 - 使用正确的字段路径
export const searchFilters = [
  {
    type: 'field_value_selection' as const,
    field: 'alert.severity',
    name: 'Severity',
    multiSelect: 'or' as const,
    options: [
      { value: 'critical', view: <EuiHealth color="danger">Critical</EuiHealth> },
      { value: 'high', view: <EuiHealth color="warning">High</EuiHealth> },
      { value: 'medium', view: <EuiHealth color="primary">Medium</EuiHealth> },
      { value: 'low', view: <EuiHealth color="success">Low</EuiHealth> },
    ],
  },
  {
    type: 'field_value_selection' as const,
    field: 'alert.evidence.event_type',
    name: 'Event Type',
    multiSelect: 'or' as const,
    options: [
      { value: 'file_access', view: 'File Access' },
      { value: 'process_execution', view: 'Process Execution' },
      { value: 'network_connection', view: 'Network Connection' },
      { value: 'system_call', view: 'System Call' },
    ],
  },
  {
    type: 'field_value_toggle' as const,
    field: 'alert.risk_score',
    value: '>=80',
    name: 'High Risk (≥80)',
  },
];

// Index选择器选项配置
export const indexOptions = [
  {
    value: 'sysarmor-alerts*',
    inputDisplay: (
      <EuiHealth color="success" style={{ lineHeight: 'inherit' }}>
        sysarmor-alerts*
      </EuiHealth>
    ),
    'data-test-subj': 'option-alerts',
  },
];
