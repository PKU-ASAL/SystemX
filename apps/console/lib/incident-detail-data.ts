export type AttackChainStep = {
  id: string;
  order: number;
  tactic: string;
  techniqueId: string;
  technique: string;
  source: string;
  target: string;
  detected: boolean;
  evidence: string;
};

export type ProvenanceNode = {
  id: string;
  node_name?: string;
  node_type?: string;
  node_desc?: string;
  node_score?: number;
  stageLevel?: "L1" | "L2" | "L3" | "L4";
  nodeVariant?: "tp" | "propagated" | "fp";
  revealAtSec?: number;
  name: string;
  type: "process" | "network" | "file";
  description: string;
  score: number;
  stage: "L1" | "L2" | "L3" | "L4";
  variant: "tp" | "propagated" | "fp";
};

export type ProvenanceEdge = {
  source: string;
  target: string;
  technique: string;
  syscall: string;
  tactic: string;
};

export type IncidentDetail = {
  incidentId: string;
  chainId: string;
  summary: string;
  attackChain: AttackChainStep[];
  provenance: {
    nodes: ProvenanceNode[];
    edges: ProvenanceEdge[];
  };
  evidence: Array<{
    time: string;
    relativeTimeSec: number;
    host: string;
    process: string;
    syscall: string;
    args: string;
    result: "success" | "fail";
    category: "network" | "file" | "process";
    detail: string;
  }>;
};

const nodeRevealAtSec: Record<string, number> = {
  "n-atk": 64,
  "n-nginx": 65,
  "n-webshell": 105,
  "n-bash": 107,
  "n-c2": 122,
  "n-id-rsa": 149,
  "n-jump": 12,
  "n-powershell": 18,
  "n-lsass": 31,
  "n-ad": 47,
};

const nodeTypeCode = {
  process: "1",
  network: "2",
  file: "3",
} as const;

const incidentDetails: IncidentDetail[] = [
  {
    incidentId: "inc-1027",
    chainId: "threat-chain-001",
    summary:
      "External attacker exploited OA web service, dropped a web shell, established C2, persisted through cron, and accessed SSH credentials.",
    attackChain: [
      {
        id: "step-initial-access",
        order: 1,
        tactic: "Initial Access",
        techniqueId: "T1190",
        technique: "Exploit Public-Facing Application",
        source: "203.0.113.42",
        target: "oa-web",
        detected: true,
        evidence: "nginx accepted exploit traffic before web shell write.",
      },
      {
        id: "step-execution",
        order: 2,
        tactic: "Execution",
        techniqueId: "T1059.004",
        technique: "Unix Shell",
        source: "nginx",
        target: "bash",
        detected: true,
        evidence: "nginx spawned bash with suspicious parent-child lineage.",
      },
      {
        id: "step-c2",
        order: 3,
        tactic: "Command and Control",
        techniqueId: "T1071.001",
        technique: "Web Protocols",
        source: ".sysupd",
        target: "10.0.0.5:443",
        detected: true,
        evidence: "Hidden process opened periodic HTTPS beacon.",
      },
      {
        id: "step-credential",
        order: 4,
        tactic: "Credential Access",
        techniqueId: "T1552.004",
        technique: "Private Keys",
        source: "cat",
        target: "id_rsa",
        detected: true,
        evidence: "Process read administrator SSH private key.",
      },
      {
        id: "step-lateral",
        order: 5,
        tactic: "Lateral Movement",
        techniqueId: "T1021.004",
        technique: "SSH",
        source: "ssh",
        target: "jump-server:22",
        detected: true,
        evidence: "SSH client connected to jump server after credential access.",
      },
    ],
    provenance: {
      nodes: [
        {
          id: "n-atk",
          name: "203.0.113.42",
          type: "network",
          description: "External scanning source",
          score: 30,
          stage: "L1",
          variant: "fp",
        },
        {
          id: "n-nginx",
          name: "nginx",
          type: "process",
          description: "Web service process",
          score: 72,
          stage: "L1",
          variant: "tp",
        },
        {
          id: "n-webshell",
          name: "img.jsp",
          type: "file",
          description: "Dropped JSP web shell",
          score: 95,
          stage: "L1",
          variant: "tp",
        },
        {
          id: "n-bash",
          name: "bash",
          type: "process",
          description: "Malicious shell spawned by nginx",
          score: 98,
          stage: "L2",
          variant: "tp",
        },
        {
          id: "n-c2",
          name: "10.0.0.5:443",
          type: "network",
          description: "HTTPS C2 beacon",
          score: 99,
          stage: "L2",
          variant: "tp",
        },
        {
          id: "n-id-rsa",
          name: "id_rsa",
          type: "file",
          description: "Administrator SSH private key",
          score: 96,
          stage: "L3",
          variant: "tp",
        },
      ],
      edges: [
        { source: "n-atk", target: "n-nginx", technique: "T1190", syscall: "connect", tactic: "InitialAccess" },
        { source: "n-nginx", target: "n-webshell", technique: "T1505.003", syscall: "write", tactic: "Persistence" },
        { source: "n-nginx", target: "n-bash", technique: "T1059.004", syscall: "execve", tactic: "Execution" },
        { source: "n-bash", target: "n-c2", technique: "T1071.001", syscall: "connect", tactic: "C2" },
        { source: "n-bash", target: "n-id-rsa", technique: "T1552.004", syscall: "read", tactic: "CredentialAccess" },
      ],
    },
    evidence: [
      {
        time: "21:03:18",
        relativeTimeSec: 105,
        host: "oa-web",
        process: "nginx",
        syscall: "write",
        args: "/var/www/html/upload/img.jsp",
        result: "success",
        category: "file",
        detail: "Created /var/www/html/upload/img.jsp",
      },
      {
        time: "21:03:24",
        relativeTimeSec: 107,
        host: "oa-web",
        process: "bash",
        syscall: "execve",
        args: ".sysupd",
        result: "success",
        category: "process",
        detail: "Spawned hidden implant .sysupd",
      },
      {
        time: "21:03:31",
        relativeTimeSec: 122,
        host: "oa-web",
        process: ".sysupd",
        syscall: "connect",
        args: "10.0.0.5:443",
        result: "success",
        category: "network",
        detail: "Connected to 10.0.0.5:443",
      },
    ],
  },
  {
    incidentId: "inc-1019",
    chainId: "threat-chain-002",
    summary:
      "Credential theft sequence started from a jump server, reached directory services, and left suspicious authentication artifacts.",
    attackChain: [
      {
        id: "step-execution-002",
        order: 1,
        tactic: "Execution",
        techniqueId: "T1059",
        technique: "Command and Scripting Interpreter",
        source: "jump-server",
        target: "powershell",
        detected: true,
        evidence: "Interactive shell launched credential discovery script.",
      },
      {
        id: "step-credential-002",
        order: 2,
        tactic: "Credential Access",
        techniqueId: "T1003",
        technique: "OS Credential Dumping",
        source: "powershell",
        target: "lsass",
        detected: true,
        evidence: "Memory read pattern matched credential dumping behavior.",
      },
      {
        id: "step-discovery-002",
        order: 3,
        tactic: "Discovery",
        techniqueId: "T1087",
        technique: "Account Discovery",
        source: "jump-server",
        target: "ad-controller",
        detected: true,
        evidence: "Burst of LDAP queries against privileged groups.",
      },
    ],
    provenance: {
      nodes: [
        {
          id: "n-jump",
          name: "jump-server",
          type: "process",
          description: "Compromised remote session host",
          score: 82,
          stage: "L1",
          variant: "tp",
        },
        {
          id: "n-powershell",
          name: "powershell",
          type: "process",
          description: "Credential discovery script runner",
          score: 91,
          stage: "L2",
          variant: "tp",
        },
        {
          id: "n-lsass",
          name: "lsass",
          type: "process",
          description: "Credential material access target",
          score: 94,
          stage: "L2",
          variant: "tp",
        },
        {
          id: "n-ad",
          name: "ad-controller",
          type: "network",
          description: "Directory service target",
          score: 78,
          stage: "L3",
          variant: "propagated",
        },
      ],
      edges: [
        { source: "n-jump", target: "n-powershell", technique: "T1059", syscall: "execve", tactic: "Execution" },
        { source: "n-powershell", target: "n-lsass", technique: "T1003", syscall: "read", tactic: "CredentialAccess" },
        { source: "n-powershell", target: "n-ad", technique: "T1087", syscall: "connect", tactic: "Discovery" },
      ],
    },
    evidence: [
      {
        time: "20:51:02",
        relativeTimeSec: 18,
        host: "jump-server",
        process: "powershell",
        syscall: "execve",
        args: "Invoke-CredAudit.ps1",
        result: "success",
        category: "process",
        detail: "Executed credential collection script",
      },
      {
        time: "20:51:16",
        relativeTimeSec: 31,
        host: "jump-server",
        process: "powershell",
        syscall: "read",
        args: "lsass memory",
        result: "success",
        category: "process",
        detail: "Read credential-bearing process memory",
      },
    ],
  },
];

export function getIncidentDetail(incidentId: string) {
  const detail = incidentDetails.find((item) => item.incidentId === incidentId);

  if (!detail) return undefined;

  return {
    ...detail,
    provenance: {
      ...detail.provenance,
      nodes: detail.provenance.nodes.map(normalizeProvenanceNode),
    },
  };
}

export function getSyscallsForProvenanceNode(detail: IncidentDetail, node: ProvenanceNode) {
  const revealAtSec = node.revealAtSec ?? 0;
  const windowSeconds = 35;

  return detail.evidence.filter((event) => {
    if (Math.abs(event.relativeTimeSec - revealAtSec) > windowSeconds) return false;
    if (node.type === "process") return event.process === node.name || event.args.includes(node.name);
    if (node.type === "file") return event.args.includes(node.name) || event.detail.includes(node.name);
    if (node.type === "network") return event.category === "network" && event.args.includes(node.name);

    return true;
  });
}

export function getSyscallsForAttackStep(detail: IncidentDetail, step: AttackChainStep) {
  const target = step.target.toLowerCase();
  const source = step.source.toLowerCase();
  const matches = (event: IncidentDetail["evidence"][number], token: string) => {
    const haystack = `${event.process} ${event.args} ${event.detail}`.toLowerCase();

    return Boolean(token && haystack.includes(token));
  };
  const targetMatches = detail.evidence.filter((event) => matches(event, target));

  return targetMatches.length > 0 ? targetMatches : detail.evidence.filter((event) => matches(event, source));
}

function normalizeProvenanceNode(node: ProvenanceNode): ProvenanceNode {
  return {
    ...node,
    node_name: node.node_name ?? node.name,
    node_type: node.node_type ?? nodeTypeCode[node.type],
    node_desc: node.node_desc ?? node.description,
    node_score: node.node_score ?? node.score,
    stageLevel: node.stageLevel ?? node.stage,
    nodeVariant: node.nodeVariant ?? node.variant,
    revealAtSec: node.revealAtSec ?? nodeRevealAtSec[node.id],
  };
}
