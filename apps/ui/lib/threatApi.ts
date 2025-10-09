// 威胁图谱API服务层
import axios, { AxiosResponse } from 'axios';
import { getThreatApiConfig, isThreatApiEnabled } from './externalApiConfig';
import {
  APIResponse,
  NodeInfoAPIResponse,
  ThreatNode,
  ThreatEdge,
  ThreatGraphData,
  ProcessedHop,
  NodeInfo,
  NetworkConnection,
  TimelineEntry,
  NetworkTopology
} from '../types/threat';

export class ThreatAPI {
  private static cache = new Map<string, ThreatGraphData>();
  private static nodeInfoCache = new Map<string, NodeInfo>();

  // 获取API基础URL - 使用配置管理器
  private static getBaseUrl(): string {
    const config = getThreatApiConfig();
    return config.baseUrl;
  }

  // 检查API是否启用
  private static isEnabled(): boolean {
    return isThreatApiEnabled();
  }

  /**
   * 获取威胁图数据 - 主要API
   */
  static async getThreatGraphData(threatId: string): Promise<ThreatGraphData> {
    console.group(`🎯 [THREAT-API] 获取威胁图数据: ${threatId}`);

    try {
      // 检查缓存
      if (this.cache.has(threatId)) {
        console.log(`📦 [CACHE] 从缓存获取威胁数据: ${threatId}`);
        console.groupEnd();
        return this.cache.get(threatId)!;
      }

      // 检查API是否启用
      if (!this.isEnabled()) {
        throw new Error('威胁图谱API功能已禁用');
      }

      console.log(`📡 [API] 调用威胁图API (增强版 - 支持边标签)`);
      const config = getThreatApiConfig();
      const url = `${config.baseUrl}/alert/alert_chain_new_new_new`;

      const response: AxiosResponse<APIResponse> = await axios.get(url, {
        params: { threat_id: threatId },
        timeout: config.timeout,
        headers: config.headers
      });

      console.log(`✅ [API_SUCCESS] API响应:`, {
        code: response.data.code,
        message: response.data.message,
        hasData: !!response.data.data
      });

      // 验证API响应格式
      if (response.data.code !== '0000' || response.data.message !== 'success') {
        throw new Error(`API错误: ${response.data.message} (${response.data.code})`);
      }

      if (!response.data.data || !Array.isArray(response.data.data) || response.data.data.length === 0) {
        throw new Error(`威胁 ${threatId} 数据为空`);
      }

      // 解析data[0]中的JSON字符串
      let parsedGraphData: { nodes: ThreatNode[]; edges: ThreatEdge[] };
      try {
        const jsonString = response.data.data[0];
        if (typeof jsonString === 'string') {
          parsedGraphData = JSON.parse(jsonString);
        } else {
          parsedGraphData = jsonString;
        }
      } catch (parseError) {
        throw new Error(`JSON解析失败: ${parseError}`);
      }

      // 处理威胁数据
      console.log(`🔄 [PROCESS] 开始处理威胁数据`);
      console.log(`🔍 [RAW-EDGES] 原始边数据样本:`, {
        totalEdges: parsedGraphData.edges?.length || 0,
        sampleEdges: parsedGraphData.edges?.slice(0, 2).map(edge => ({
          source: edge.source || edge.from || edge.src,
          target: edge.target || edge.to || edge.dst,
          technique: edge.technique,
          syscall: edge.syscall,
          tactic: edge.tactic,
          timestamp: edge.timestamp,
          allFields: Object.keys(edge)
        }))
      });
      const processedData = this.processGraphData(threatId, parsedGraphData);

      // 缓存结果
      this.cache.set(threatId, processedData);

      console.log(`✅ [SUCCESS] 威胁数据处理完成`);
      console.groupEnd();

      return processedData;

    } catch (error) {
      console.error(`❌ [ERROR] 获取威胁图数据失败:`, error);
      console.groupEnd();
      throw error;
    }
  }

  /**
   * 获取节点详细信息
   */
  static async getNodeInfo(threatId: string, nodeId: string): Promise<NodeInfo | null> {
    const cacheKey = `${threatId}_${nodeId}`;

    try {
      // 检查缓存
      if (this.nodeInfoCache.has(cacheKey)) {
        console.log(`📦 [CACHE] 从缓存获取节点信息: ${cacheKey}`);
        return this.nodeInfoCache.get(cacheKey)!;
      }

      console.log(`📡 [API] 调用节点信息API: ${threatId} + ${nodeId}`);

      const config = getThreatApiConfig();
      const url = `${config.baseUrl}/alert/node_info`;
      const response: AxiosResponse<NodeInfoAPIResponse> = await axios.get(url, {
        params: {
          threat_id: threatId,
          node_id: nodeId
        },
        timeout: config.timeout || 5000,
        headers: config.headers
      });

      // 验证响应格式：code: 200, msg: "success"
      if (response.data.code !== 200 || response.data.msg !== 'success') {
        console.warn(`⚠️ [API_WARNING] 节点信息API错误: ${response.data.msg} (${response.data.code})`);
        return null;
      }

      const nodeInfo: NodeInfo = {
        code: response.data.code,
        msg: response.data.msg,
        data: response.data.data
      };

      // 缓存结果
      this.nodeInfoCache.set(cacheKey, nodeInfo);

      console.log(`✅ [SUCCESS] 节点信息获取成功: ${nodeId}`);
      console.log(`📋 [NODE_FIELDS] 节点字段:`, Object.keys(response.data.data || {}));

      return nodeInfo;

    } catch (error) {
      console.error(`❌ [ERROR] 获取节点信息失败:`, error);
      return null;
    }
  }

  /**
   * 获取威胁列表
   */
  static async getThreatList(): Promise<string[]> {
    try {
      console.log(`📋 [THREAT_LIST] 开始动态探测威胁ID列表`);

      const availableThreatIds: string[] = [];
      const config = getThreatApiConfig();
      const url = `${config.baseUrl}/alert/alert_chain_new_new_new`;
      
      let consecutiveFailures = 0;
      const maxConsecutiveFailures = 10; // 增加到连续10个ID失败后停止
      let idNumber = 1;
      
      // 从th-001开始逐个探测，直到连续失败多次
      while (true) {
        const threatId = `th-${String(idNumber).padStart(3, '0')}`;
        
        try {
          const response = await axios.get(url, {
            params: { threat_id: threatId },
            timeout: 3000, // 3秒超时
            headers: config.headers
          });

          // 检查是否有有效数据
          if (response.data.code === '0000' &&
            response.data.data &&
            Array.isArray(response.data.data) &&
            response.data.data.length > 0) {
            availableThreatIds.push(threatId);
            consecutiveFailures = 0; // 重置连续失败计数
            console.log(`✅ [THREAT_TEST] 发现可用威胁ID: ${threatId}`);
          } else {
            consecutiveFailures++;
          }
        } catch (error) {
          // 请求失败，增加连续失败计数
          consecutiveFailures++;
          console.log(`❌ [THREAT_TEST] 威胁ID ${threatId} 探测失败: ${error instanceof Error ? error.message : '未知错误'}`);
        }
        
        // 如果连续失败太多次，停止探测
        if (consecutiveFailures >= maxConsecutiveFailures) {
          console.log(`🛑 [THREAT_TEST] 连续${maxConsecutiveFailures}个ID未找到，停止探测`);
          break;
        }
        
        // 安全限制：最多探测1000个ID
        if (idNumber >= 1000) {
          console.log(`⚠️ [THREAT_TEST] 已达到最大探测限制(1000个ID)`);
          break;
        }
        
        idNumber++;
      }

      console.log(`✅ [THREAT_LIST] 探测完成，发现 ${availableThreatIds.length} 个可用威胁ID:`, availableThreatIds);

      // 如果没有找到任何可用的威胁ID，返回空数组
      if (availableThreatIds.length === 0) {
        console.log(`⚠️ [THREAT_LIST] 未找到任何可用威胁ID`);
        return [];
      }

      return availableThreatIds.sort();

    } catch (error) {
      console.error(`❌ [THREAT_LIST] 获取威胁列表失败:`, error);
      // 返回空数组
      return [];
    }
  }

  /**
   * 获取威胁PDF报告
   */
  static async getThreatReportPdf(threatId: string): Promise<{
    pdf_base64: string;
    filename?: string;
  }> {
    try {
      console.log(`📄 [PDF] 获取威胁PDF报告: ${threatId}`);

      if (!this.isEnabled()) {
        throw new Error('威胁图谱API功能已禁用');
      }

      const config = getThreatApiConfig();
      const url = `${config.baseUrl}/alert/alert_pdf`;
      
      const response = await axios.get(url, {
        params: { threat_id: threatId },
        timeout: config.timeout || 30000, // PDF生成可能需要更长时间
        headers: config.headers
      });

      console.log(`✅ [PDF] PDF报告API响应:`, {
        code: response.data.code,
        message: response.data.message,
        hasData: !!response.data.data
      });

      // 验证API响应格式
      if (response.data.code !== '0000' || response.data.message !== 'success') {
        throw new Error(`PDF报告API错误: ${response.data.message} (${response.data.code})`);
      }

      if (!response.data.data || !response.data.data.pdf_base64) {
        throw new Error(`威胁 ${threatId} PDF报告数据为空`);
      }

      return {
        pdf_base64: response.data.data.pdf_base64,
        filename: `threat-report-${threatId}.pdf`
      };

    } catch (error) {
      console.error(`❌ [PDF] 获取威胁PDF报告失败:`, error);
      throw error;
    }
  }

  /**
   * 测试API连接
   */
  static async testConnection(): Promise<{ success: boolean; message: string }> {
    try {
      console.log(`🔗 [TEST] 测试API连接`);

      const config = getThreatApiConfig();
      const response = await axios.get(`${config.baseUrl}/alert/alert_chain_new_new`, {
        params: { threat_id: 'th-001' },
        timeout: config.timeout || 5000,
        headers: config.headers
      });

      if (response.data && (response.data.code === '0000' || response.data.code === 200)) {
        console.log(`✅ [TEST_SUCCESS] API连接正常`);
        return { success: true, message: 'API连接正常' };
      } else {
        console.log(`⚠️ [TEST_WARNING] API响应异常:`, response.data);
        return { success: false, message: `API响应异常` };
      }

    } catch (error) {
      console.error(`❌ [TEST_ERROR] API连接失败:`, error);
      return { success: false, message: `API连接失败: ${(error as Error).message}` };
    }
  }

  /**
   * 处理原始图数据
   */
  private static processGraphData(threatId: string, rawData: { nodes: ThreatNode[]; edges: ThreatEdge[] }): ThreatGraphData {
    console.log(`🔄 [PROCESS] 开始处理威胁图数据`);
    console.log(`📋 [RAW_DATA] 原始数据格式:`, {
      hasNodes: !!rawData.nodes,
      hasEdges: !!rawData.edges,
      nodeCount: rawData.nodes ? rawData.nodes.length : 0,
      edgeCount: rawData.edges ? rawData.edges.length : 0
    });

    // 将nodes转换为hop序列格式
    let hopSequence: ProcessedHop[] = [];

    if (rawData.nodes && Array.isArray(rawData.nodes)) {
      hopSequence = rawData.nodes.map((node: ThreatNode, index: number) => ({
        hop_id: index,
        depth: 0, // 初始设为0，后续通过边分析深度
        path: String(index),
        node_id: node.id || String(index),
        node_desc: node.node_desc || node.description || `节点 ${node.id}`,
        node_name: node.node_name || node.name || '',
        node_label: node.node_label || node.label || '',
        node_type: node.node_type || node.type || '',
        node_score: node.node_score || node.score || '',
        node_source: node.node_source || node.source || '',
        is_abstract: node.node_abstract === '1' || node.node_abstract === true || node.is_abstract === true,
        timestamps: this.extractTimestamps(node.node_desc || ''),
        network_connections: this.extractNetworkConnections(node.node_desc || ''),
        children_count: 0,
        originalNode: node
      }));

      // 根据edges分析层次结构
      if (rawData.edges && Array.isArray(rawData.edges)) {
        this.analyzeDepthFromEdges(hopSequence, rawData.edges);
      }
    }

    console.log(`📊 [HOP_EXTRACT] 提取到 ${hopSequence.length} 个hop`);

    // 分析深度分布
    const depthDistribution: { [depth: number]: number } = {};
    let maxDepth = 0;

    hopSequence.forEach(hop => {
      const depth = hop.depth || 0;
      depthDistribution[depth] = (depthDistribution[depth] || 0) + 1;
      maxDepth = Math.max(maxDepth, depth);
    });

    // 识别第一层节点 (depth=0)
    const firstLayerNodes = hopSequence
      .filter(hop => hop.depth === 0)
      .map((hop, index) => `node_${hop.hop_id !== undefined ? hop.hop_id : index}`);

    console.log(`🎯 [FIRST_LAYER] 识别到 ${firstLayerNodes.length} 个第一层节点`);

    // 统计时间戳信息
    const timestampedHops = hopSequence.filter(hop => hop.timestamps && hop.timestamps.length > 0);
    const nonTimestampedHops = hopSequence.filter(hop => !hop.timestamps || hop.timestamps.length === 0);

    // 创建时间线数据
    const timelineData = this.createTimelineData(hopSequence);

    // 生成网络拓扑
    const networkTopology = this.generateNetworkTopology(hopSequence);

    const processedData: ThreatGraphData = {
      threat_id: threatId,
      hop_sequence: hopSequence,
      // 添加nodes字段以兼容AttackTimelineCytoscape
      nodes: hopSequence.map(hop => ({
        id: hop.node_id,
        node_desc: hop.node_desc,
        node_name: hop.node_name,
        node_label: hop.node_label,
        node_type: hop.originalNode?.node_type,
        node_score: hop.originalNode?.node_score,
        node_source: hop.originalNode?.node_source,
        node_abstract: hop.originalNode?.node_abstract,
        node_start_time: hop.originalNode?.node_start_time,
        node_end_time: hop.originalNode?.node_end_time
      })),
      // 添加edges字段以兼容
      edges: rawData.edges || [],
      max_depth: maxDepth,
      bfs_analysis: {
        total_hops: hopSequence.length,
        depth_distribution: depthDistribution,
        timestamped_hops: timestampedHops.length,
        non_timestamped_hops: nonTimestampedHops.length,
        first_layer_nodes: firstLayerNodes
      },
      timeline_data: timelineData,
      network_topology: networkTopology,
      metadata: {
        created_at: new Date().toISOString(),
        status: 'active',
        severity: 'medium',
        originalEdges: rawData.edges || []
      }
    };

    console.log(`✅ [PROCESS_SUCCESS] 威胁数据处理完成:`, {
      威胁ID: processedData.threat_id,
      最大深度: processedData.max_depth,
      总hop数: processedData.bfs_analysis.total_hops,
      第一层节点数: processedData.bfs_analysis.first_layer_nodes.length,
      有时间戳hop: processedData.bfs_analysis.timestamped_hops,
      网络节点数: processedData.network_topology.nodes.length
    });

    return processedData;
  }

  /**
   * 根据边关系分析节点深度 (BFS)
   */
  private static analyzeDepthFromEdges(hopSequence: ProcessedHop[], edges: ThreatEdge[]): void {
    console.log(`🔄 [DEPTH_ANALYSIS] 分析 ${edges.length} 条边的深度关系`);

    // 构建节点ID到索引的映射
    const nodeIdToIndex = new Map<string, number>();
    hopSequence.forEach((hop, index) => {
      const nodeId = hop.node_id || hop.originalNode?.id || String(index);
      nodeIdToIndex.set(nodeId, index);
    });

    // 构建图的邻接表
    const adjacencyList = new Map<string, string[]>();
    const inDegree = new Map<string, number>();

    // 初始化
    hopSequence.forEach(hop => {
      const nodeId = hop.node_id || hop.originalNode?.id || String(hop.hop_id);
      adjacencyList.set(nodeId, []);
      inDegree.set(nodeId, 0);
    });

    // 构建边关系
    edges.forEach(edge => {
      const fromId = edge.from || edge.source || edge.src;
      const toId = edge.to || edge.target || edge.dst;

      if (fromId && toId) {
        if (!adjacencyList.has(fromId)) {
          adjacencyList.set(fromId, []);
          inDegree.set(fromId, 0);
        }
        if (!adjacencyList.has(toId)) {
          adjacencyList.set(toId, []);
          inDegree.set(toId, 0);
        }

        adjacencyList.get(fromId)?.push(toId);
        inDegree.set(toId, (inDegree.get(toId) || 0) + 1);
      }
    });

    // 使用BFS计算深度
    const depths = new Map<string, number>();

    // 找到入度为0的节点作为根节点（深度0）
    const rootNodes: string[] = [];
    for (const [nodeId, degree] of inDegree.entries()) {
      if (degree === 0) {
        rootNodes.push(nodeId);
        depths.set(nodeId, 0);
      }
    }

    // 如果没有找到根节点，选择第一个节点作为根
    if (rootNodes.length === 0 && hopSequence.length > 0) {
      const firstNodeId = hopSequence[0].node_id || hopSequence[0].originalNode?.id || '0';
      rootNodes.push(firstNodeId);
      depths.set(firstNodeId, 0);
    }

    // BFS计算深度
    const queue: string[] = [...rootNodes];
    const visited = new Set<string>();

    while (queue.length > 0) {
      const currentId = queue.shift()!;
      const currentDepth = depths.get(currentId) || 0;
      visited.add(currentId);

      const neighbors = adjacencyList.get(currentId) || [];
      for (const neighborId of neighbors) {
        if (!depths.has(neighborId) || depths.get(neighborId)! > currentDepth + 1) {
          depths.set(neighborId, currentDepth + 1);
          if (!visited.has(neighborId)) {
            queue.push(neighborId);
          }
        }
      }
    }

    // 应用深度到hop序列
    let updatedCount = 0;
    hopSequence.forEach((hop, index) => {
      const nodeId = hop.node_id || hop.originalNode?.id || String(index);
      const calculatedDepth = depths.get(nodeId);

      if (calculatedDepth !== undefined) {
        hop.depth = calculatedDepth;
        updatedCount++;
      } else {
        hop.depth = 0;
      }
    });

    console.log(`✅ [DEPTH_ANALYSIS] 深度分析完成: ${updatedCount}/${hopSequence.length} 个节点更新深度`);
  }

  /**
   * 提取时间戳
   */
  private static extractTimestamps(text: string): string[] {
    const timestamps: string[] = [];
    const patterns = [
      /\d{4}-\d{2}-\d{2}[\sT]\d{2}:\d{2}:\d{2}/g,
      /\d{4}\/\d{2}\/\d{2}\s\d{2}:\d{2}:\d{2}/g,
      /\d{10,13}/g,
      /\d{2}:\d{2}:\d{2}/g,
    ];

    for (const pattern of patterns) {
      const matches = (text || '').match(pattern);
      if (matches) {
        timestamps.push(...matches);
      }
    }

    return timestamps;
  }

  /**
   * 提取网络连接
   */
  private static extractNetworkConnections(text: string): NetworkConnection[] {
    const pattern = /(\d+\.\d+\.\d+\.\d+):(\d+)->(\d+\.\d+\.\d+\.\d+):(\d+)/g;
    const connections: NetworkConnection[] = [];
    let match;

    while ((match = pattern.exec(text || '')) !== null) {
      connections.push({
        source_ip: match[1],
        source_port: parseInt(match[2]),
        dest_ip: match[3],
        dest_port: parseInt(match[4]),
        connection_string: `${match[1]}:${match[2]}->${match[3]}:${match[4]}`
      });
    }

    return connections;
  }

  /**
   * 创建时间线数据
   */
  private static createTimelineData(hopSequence: ProcessedHop[]): TimelineEntry[] {
    return hopSequence.map(hop => ({
      hop_id: hop.hop_id,
      depth: hop.depth,
      timestamp_type: (hop.timestamps && hop.timestamps.length > 0) ? 'extracted' : 'sequence',
      timestamps: (hop.timestamps && hop.timestamps.length > 0) ? hop.timestamps : [`step_${hop.hop_id.toString().padStart(3, '0')}`],
      description: hop.node_desc,
      node_type: hop.node_type || '',
      node_label: hop.node_label || '',
      network_connections: hop.network_connections,
      is_abstract: hop.is_abstract
    }));
  }

  /**
   * 生成网络拓扑数据
   */
  private static generateNetworkTopology(hopSequence: ProcessedHop[]): NetworkTopology {
    const nodes: { [ip: string]: any } = {};
    const edges: any[] = [];

    hopSequence.forEach(hop => {
      hop.network_connections?.forEach((conn: NetworkConnection) => {
        const srcIp = conn.source_ip;
        const dstIp = conn.dest_ip;

        // 添加节点
        if (!nodes[srcIp]) {
          nodes[srcIp] = {
            ip: srcIp,
            type: this.isInternalIp(srcIp) ? 'internal' : 'external',
            connections_out: 0,
            connections_in: 0
          };
        }

        if (!nodes[dstIp]) {
          nodes[dstIp] = {
            ip: dstIp,
            type: this.isInternalIp(dstIp) ? 'internal' : 'external',
            connections_out: 0,
            connections_in: 0
          };
        }

        // 统计连接
        nodes[srcIp].connections_out += 1;
        nodes[dstIp].connections_in += 1;

        // 添加边
        edges.push({
          source: srcIp,
          target: dstIp,
          source_port: conn.source_port,
          target_port: conn.dest_port,
          hop_id: hop.hop_id
        });
      });
    });

    return {
      nodes: Object.values(nodes),
      edges: edges,
      stats: {
        total_nodes: Object.keys(nodes).length,
        total_edges: edges.length,
        internal_nodes: Object.values(nodes).filter((n: any) => n.type === 'internal').length,
        external_nodes: Object.values(nodes).filter((n: any) => n.type === 'external').length,
      }
    };
  }

  /**
   * 判断是否为内网IP
   */
  private static isInternalIp(ip: string): boolean {
    const internalRanges = [
      /^10\./,
      /^172\.(1[6-9]|2[0-9]|3[0-1])\./,
      /^192\.168\./,
      /^127\./
    ];

    return internalRanges.some(range => range.test(ip));
  }

  /**
   * 清除缓存
   */
  static clearCache(): void {
    this.cache.clear();
    this.nodeInfoCache.clear();
    console.log(`🗑️ [CACHE] 缓存已清除`);
  }
}
