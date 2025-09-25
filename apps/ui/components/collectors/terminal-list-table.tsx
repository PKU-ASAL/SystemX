"use client";

import * as React from "react";
import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type UniqueIdentifier,
} from "@dnd-kit/core";
import { restrictToVerticalAxis } from "@dnd-kit/modifiers";
import {
  arrayMove,
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  IconChevronDown,
  IconChevronLeft,
  IconChevronRight,
  IconChevronsLeft,
  IconChevronsRight,
  IconDotsVertical,
  IconGripVertical,
  IconLayoutColumns,
  IconPlus,
  IconRefresh,
  IconTrash,
  IconEye,
} from "@tabler/icons-react";
import {
  ColumnDef,
  ColumnFiltersState,
  flexRender,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  Row,
  SortingState,
  useReactTable,
  VisibilityState,
} from "@tanstack/react-table";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Copy } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { DeploymentInstructions } from "@/components/collectors/deployment-instructions";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { apiClient, type Collector } from "@/lib/api";

// Create a separate component for the drag handle
function DragHandle({ id }: { id: string }) {
  const { attributes, listeners } = useSortable({
    id,
  });

  return (
    <Button
      {...attributes}
      {...listeners}
      variant="ghost"
      size="icon"
      className="text-muted-foreground size-7 hover:bg-transparent"
    >
      <IconGripVertical className="text-muted-foreground size-3" />
      <span className="sr-only">Drag to reorder</span>
    </Button>
  );
}

function DraggableRow({ row }: { row: Row<Collector> }) {
  const { transform, transition, setNodeRef, isDragging } = useSortable({
    id: row.original.collector_id,
  });

  return (
    <TableRow
      data-state={row.getIsSelected() && "selected"}
      data-dragging={isDragging}
      ref={setNodeRef}
      className="relative z-0 data-[dragging=true]:z-10 data-[dragging=true]:opacity-80"
      style={{
        transform: CSS.Transform.toString(transform),
        transition: transition,
      }}
    >
      {row.getVisibleCells().map((cell) => (
        <TableCell key={cell.id}>
          {flexRender(cell.column.columnDef.cell, cell.getContext())}
        </TableCell>
      ))}
    </TableRow>
  );
}

export function TerminalListTable() {
  const [collectors, setCollectors] = React.useState<Collector[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [searchTerm, setSearchTerm] = React.useState("");
  const [statusFilter, setStatusFilter] = React.useState<string>("all");
  const [environmentFilter, setEnvironmentFilter] =
    React.useState<string>("all");
  const [rowSelection, setRowSelection] = React.useState({});
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    []
  );
  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [pagination, setPagination] = React.useState({
    pageIndex: 0,
    pageSize: 10,
  });

  // 对话框状态
  const [detailDialogOpen, setDetailDialogOpen] = React.useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = React.useState(false);
  const [selectedCollector, setSelectedCollector] =
    React.useState<Collector | null>(null);

  const sortableId = React.useId();
  const sensors = useSensors(
    useSensor(MouseSensor, {}),
    useSensor(TouchSensor, {}),
    useSensor(KeyboardSensor, {})
  );

  const dataIds = React.useMemo<UniqueIdentifier[]>(
    () => collectors?.map(({ collector_id }) => collector_id) || [],
    [collectors]
  );

  const fetchCollectors = async () => {
    try {
      setLoading(true);
      const params: any = {
        page: pagination.pageIndex + 1,
        limit: pagination.pageSize,
      };

      if (statusFilter !== "all") {
        params.status = statusFilter;
      }

      if (environmentFilter !== "all") {
        params.environment = environmentFilter;
      }

      const response = await apiClient.getCollectors(params);
      setCollectors(response.data || []);
    } catch (error) {
      console.error("Failed to fetch collectors:", error);
      setCollectors([]);
    } finally {
      setLoading(false);
    }
  };

  React.useEffect(() => {
    fetchCollectors();
  }, [
    statusFilter,
    environmentFilter,
    pagination.pageIndex,
    pagination.pageSize,
  ]);

  const handleConfirmDelete = async () => {
    if (!selectedCollector) return;

    try {
      const response = await apiClient.deleteCollector(
        selectedCollector.collector_id,
        {
          force: true,
        }
      );
      if (response.success) {
        fetchCollectors();
        setDeleteDialogOpen(false);
        setSelectedCollector(null);
      } else {
        alert(`删除失败: ${response.message || "未知错误"}`);
      }
    } catch (error) {
      console.error("Failed to delete collector:", error);
      if (error instanceof Error) {
        alert(`删除失败: ${error.message}`);
      }
    }
  };

  const getStatusBadge = (status: string) => {
    const statusMap: Record<
      string,
      {
        variant: "default" | "secondary" | "destructive" | "outline";
        label: string;
      }
    > = {
      active: { variant: "default", label: "在线" },
      inactive: { variant: "destructive", label: "离线" },
      unknown: { variant: "secondary", label: "未知" },
    };

    const statusInfo = statusMap[status] || {
      variant: "outline",
      label: status,
    };
    return <Badge variant={statusInfo.variant}>{statusInfo.label}</Badge>;
  };

  const getEnvironmentBadge = (env?: string) => {
    if (!env) return null;
    const envMap: Record<
      string,
      { variant: "default" | "secondary" | "destructive" | "outline" }
    > = {
      prod: { variant: "destructive" },
      staging: { variant: "secondary" },
      dev: { variant: "outline" },
    };
    const envInfo = envMap[env] || { variant: "outline" };
    return <Badge variant={envInfo.variant}>{env}</Badge>;
  };

  const columns: ColumnDef<Collector>[] = [
    {
      id: "drag",
      header: () => null,
      cell: ({ row }) => <DragHandle id={row.original.collector_id} />,
    },
    {
      id: "select",
      header: ({ table }) => (
        <div className="flex items-center justify-center">
          <Checkbox
            checked={
              table.getIsAllPageRowsSelected() ||
              (table.getIsSomePageRowsSelected() && "indeterminate")
            }
            onCheckedChange={(value) =>
              table.toggleAllPageRowsSelected(!!value)
            }
            aria-label="Select all"
          />
        </div>
      ),
      cell: ({ row }) => (
        <div className="flex items-center justify-center">
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => row.toggleSelected(!!value)}
            aria-label="Select row"
          />
        </div>
      ),
      enableSorting: false,
      enableHiding: false,
    },
    {
      accessorKey: "collector_id",
      header: "Collector ID",
      cell: ({ row }) => (
        <div className="font-mono text-xs">
          {row.original.collector_id
            ? row.original.collector_id.substring(0, 8) + "..."
            : "N/A"}
        </div>
      ),
    },
    {
      accessorKey: "hostname",
      header: "主机名",
      cell: ({ row }) => (
        <div className="font-medium">{row.original.hostname}</div>
      ),
      enableHiding: false,
    },
    {
      accessorKey: "ip_address",
      header: "IP 地址",
      cell: ({ row }) => (
        <div className="font-mono text-sm">{row.original.ip_address}</div>
      ),
    },
    {
      accessorKey: "os_type",
      header: "操作系统",
      cell: ({ row }) => (
        <Badge variant="outline" className="text-muted-foreground px-1.5">
          {row.original.os_type} {row.original.os_version}
        </Badge>
      ),
    },
    {
      accessorKey: "status",
      header: "状态",
      cell: ({ row }) => getStatusBadge(row.original.status),
    },
    {
      accessorKey: "metadata.environment",
      header: "环境",
      cell: ({ row }) =>
        getEnvironmentBadge(row.original.metadata?.environment),
    },
    {
      accessorKey: "metadata.group",
      header: "分组",
      cell: ({ row }) =>
        row.original.metadata?.group ? (
          <Badge variant="outline">{row.original.metadata.group}</Badge>
        ) : null,
    },
    {
      accessorKey: "last_seen",
      header: "最后上线",
      cell: ({ row }) => (
        <div className="text-sm text-muted-foreground">
          {row.original.last_seen
            ? new Date(row.original.last_seen).toLocaleString()
            : "未知"}
        </div>
      ),
    },
    {
      id: "actions",
      cell: ({ row }) => (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              className="data-[state=open]:bg-muted text-muted-foreground flex size-8"
              size="icon"
            >
              <IconDotsVertical />
              <span className="sr-only">Open menu</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-32">
            <DropdownMenuItem
              onClick={() => {
                setSelectedCollector(row.original);
                setDetailDialogOpen(true);
              }}
            >
              <IconEye className="mr-2 h-4 w-4" />
              详情
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className="text-red-600 focus:text-red-600"
              onClick={() => {
                setSelectedCollector(row.original);
                setDeleteDialogOpen(true);
              }}
            >
              <IconTrash className="mr-2 h-4 w-4" />
              删除
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ];

  const table = useReactTable({
    data: collectors,
    columns,
    state: {
      sorting,
      columnVisibility,
      rowSelection,
      columnFilters,
      pagination,
    },
    getRowId: (row) => row.collector_id,
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange: setPagination,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
  });

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (active && over && active.id !== over.id) {
      setCollectors((data) => {
        const oldIndex = dataIds.indexOf(active.id);
        const newIndex = dataIds.indexOf(over.id);
        return arrayMove(data, oldIndex, newIndex);
      });
    }
  }

  return (
    <div className="flex flex-col h-full">
      {/* Page Description */}
      <div className="px-4 lg:px-6 py-4 border-b">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">终端列表</h1>
          <p className="text-muted-foreground text-sm">
            管理和监控所有 EDR Collector 实例
          </p>
        </div>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-4 px-4 lg:px-6 py-4">
        <div className="relative flex-1 max-w-sm">
          <Input
            placeholder="搜索终端..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-32">
            <SelectValue placeholder="状态" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">所有状态</SelectItem>
            <SelectItem value="active">在线</SelectItem>
            <SelectItem value="inactive">离线</SelectItem>
            <SelectItem value="unknown">未知</SelectItem>
          </SelectContent>
        </Select>
        <Select value={environmentFilter} onValueChange={setEnvironmentFilter}>
          <SelectTrigger className="w-32">
            <SelectValue placeholder="环境" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">所有环境</SelectItem>
            <SelectItem value="prod">生产</SelectItem>
            <SelectItem value="staging">测试</SelectItem>
            <SelectItem value="dev">开发</SelectItem>
          </SelectContent>
        </Select>
        <Button onClick={fetchCollectors} variant="outline" size="sm">
          <IconRefresh className="h-4 w-4 mr-2" />
          刷新
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm">
              <IconLayoutColumns />
              <span className="hidden lg:inline">自定义列</span>
              <span className="lg:hidden">列</span>
              <IconChevronDown />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            {table
              .getAllColumns()
              .filter(
                (column) =>
                  typeof column.accessorFn !== "undefined" &&
                  column.getCanHide()
              )
              .map((column) => {
                const columnNameMap: Record<string, string> = {
                  collector_id: "Collector ID",
                  hostname: "主机名",
                  ip_address: "IP 地址",
                  os_type: "操作系统",
                  status: "状态",
                  "metadata.environment": "环境",
                  "metadata.group": "分组",
                  last_seen: "最后上线",
                };

                return (
                  <DropdownMenuCheckboxItem
                    key={column.id}
                    checked={column.getIsVisible()}
                    onCheckedChange={(value) =>
                      column.toggleVisibility(!!value)
                    }
                  >
                    {columnNameMap[column.id] || column.id}
                  </DropdownMenuCheckboxItem>
                );
              })}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto px-4 lg:px-6">
        <div className="overflow-hidden rounded-lg border">
          <DndContext
            collisionDetection={closestCenter}
            modifiers={[restrictToVerticalAxis]}
            onDragEnd={handleDragEnd}
            sensors={sensors}
            id={sortableId}
          >
            <Table>
              <TableHeader className="bg-muted sticky top-0 z-10">
                {table.getHeaderGroups().map((headerGroup) => (
                  <TableRow key={headerGroup.id}>
                    {headerGroup.headers.map((header) => {
                      return (
                        <TableHead key={header.id} colSpan={header.colSpan}>
                          {header.isPlaceholder
                            ? null
                            : flexRender(
                                header.column.columnDef.header,
                                header.getContext()
                              )}
                        </TableHead>
                      );
                    })}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody className="**:data-[slot=table-cell]:first:w-8">
                {table.getRowModel().rows?.length ? (
                  <SortableContext
                    items={dataIds}
                    strategy={verticalListSortingStrategy}
                  >
                    {table.getRowModel().rows.map((row) => (
                      <DraggableRow key={row.id} row={row} />
                    ))}
                  </SortableContext>
                ) : (
                  <TableRow>
                    <TableCell
                      colSpan={columns.length}
                      className="h-24 text-center"
                    >
                      {loading ? "加载中..." : "暂无数据"}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </DndContext>
        </div>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between px-4 lg:px-6 py-4">
        <div className="text-muted-foreground hidden flex-1 text-sm lg:flex">
          {table.getFilteredSelectedRowModel().rows.length} of{" "}
          {table.getFilteredRowModel().rows.length} row(s) selected.
        </div>
        <div className="flex w-full items-center gap-8 lg:w-fit">
          <div className="hidden items-center gap-2 lg:flex">
            <Label htmlFor="rows-per-page" className="text-sm font-medium">
              每页显示
            </Label>
            <Select
              value={`${table.getState().pagination.pageSize}`}
              onValueChange={(value) => {
                table.setPageSize(Number(value));
              }}
            >
              <SelectTrigger size="sm" className="w-20" id="rows-per-page">
                <SelectValue
                  placeholder={table.getState().pagination.pageSize}
                />
              </SelectTrigger>
              <SelectContent side="top">
                {[10, 20, 30, 40, 50].map((pageSize) => (
                  <SelectItem key={pageSize} value={`${pageSize}`}>
                    {pageSize}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex w-fit items-center justify-center text-sm font-medium">
            第 {table.getState().pagination.pageIndex + 1} 页，共{" "}
            {table.getPageCount()} 页
          </div>
          <div className="ml-auto flex items-center gap-2 lg:ml-0">
            <Button
              variant="outline"
              className="hidden h-8 w-8 p-0 lg:flex"
              onClick={() => table.setPageIndex(0)}
              disabled={!table.getCanPreviousPage()}
            >
              <span className="sr-only">Go to first page</span>
              <IconChevronsLeft />
            </Button>
            <Button
              variant="outline"
              className="size-8"
              size="icon"
              onClick={() => table.previousPage()}
              disabled={!table.getCanPreviousPage()}
            >
              <span className="sr-only">Go to previous page</span>
              <IconChevronLeft />
            </Button>
            <Button
              variant="outline"
              className="size-8"
              size="icon"
              onClick={() => table.nextPage()}
              disabled={!table.getCanNextPage()}
            >
              <span className="sr-only">Go to next page</span>
              <IconChevronRight />
            </Button>
            <Button
              variant="outline"
              className="hidden size-8 lg:flex"
              size="icon"
              onClick={() => table.setPageIndex(table.getPageCount() - 1)}
              disabled={!table.getCanNextPage()}
            >
              <span className="sr-only">Go to last page</span>
              <IconChevronsRight />
            </Button>
          </div>
        </div>
      </div>

      {/* 详情对话框 */}
      <Dialog open={detailDialogOpen} onOpenChange={setDetailDialogOpen}>
        <DialogContent className="sm:max-w-[700px] max-h-[80vh] overflow-auto">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <IconEye className="h-5 w-5" />
              Collector 详细信息
            </DialogTitle>
            <DialogDescription>
              {selectedCollector?.hostname} ({selectedCollector?.ip_address})
              的完整信息
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            {selectedCollector && (
              <div className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <Label className="text-sm font-medium">Collector ID</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded font-mono text-sm">
                      {selectedCollector.collector_id}
                    </div>
                  </div>
                  <div>
                    <Label className="text-sm font-medium">主机名</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded">
                      {selectedCollector.hostname}
                    </div>
                  </div>
                  <div>
                    <Label className="text-sm font-medium">IP 地址</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded font-mono">
                      {selectedCollector.ip_address}
                    </div>
                  </div>
                  <div>
                    <Label className="text-sm font-medium">状态</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded">
                      {getStatusBadge(selectedCollector.status)}
                    </div>
                  </div>
                  <div>
                    <Label className="text-sm font-medium">操作系统</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded">
                      {selectedCollector.os_type} {selectedCollector.os_version}
                    </div>
                  </div>
                  <div>
                    <Label className="text-sm font-medium">部署类型</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded">
                      {selectedCollector.deployment_type || "未知"}
                    </div>
                  </div>
                  <div>
                    <Label className="text-sm font-medium">环境</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded">
                      {selectedCollector.metadata?.environment
                        ? getEnvironmentBadge(
                            selectedCollector.metadata.environment
                          )
                        : "未设置"}
                    </div>
                  </div>
                  <div>
                    <Label className="text-sm font-medium">分组</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded">
                      {selectedCollector.metadata?.group || "未设置"}
                    </div>
                  </div>
                  <div>
                    <Label className="text-sm font-medium">最后上线</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded">
                      {selectedCollector.last_seen
                        ? new Date(selectedCollector.last_seen).toLocaleString()
                        : "未知"}
                    </div>
                  </div>
                  <div>
                    <Label className="text-sm font-medium">创建时间</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded">
                      {selectedCollector.created_at
                        ? new Date(
                            selectedCollector.created_at
                          ).toLocaleString()
                        : "未知"}
                    </div>
                  </div>
                </div>

                {selectedCollector.metadata?.description && (
                  <div>
                    <Label className="text-sm font-medium">描述</Label>
                    <div className="mt-1 p-2 bg-gray-50 rounded">
                      {selectedCollector.metadata.description}
                    </div>
                  </div>
                )}

                {/* 使用可复用的部署指令组件 */}
                <div className="border-t pt-4">
                  <DeploymentInstructions
                    collectorId={selectedCollector.collector_id}
                  />
                </div>

                <div>
                  <Label className="text-sm font-medium">完整 JSON 数据</Label>
                  <div className="mt-1 p-4 bg-gray-50 rounded-lg max-h-64 overflow-auto">
                    <pre className="text-xs whitespace-pre-wrap">
                      {JSON.stringify(selectedCollector, null, 2)}
                    </pre>
                  </div>
                </div>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button onClick={() => setDetailDialogOpen(false)}>关闭</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 删除确认对话框 */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent className="sm:max-w-[400px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-red-600">
              <IconTrash className="h-5 w-5" />
              确认删除
            </DialogTitle>
            <DialogDescription>
              此操作将永久删除该 Collector，且无法撤销。
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            {selectedCollector && (
              <div className="space-y-2">
                <div className="p-3 bg-red-50 border border-red-200 rounded-lg">
                  <div className="text-sm">
                    <div>
                      <strong>主机名:</strong> {selectedCollector.hostname}
                    </div>
                    <div>
                      <strong>IP 地址:</strong> {selectedCollector.ip_address}
                    </div>
                    <div>
                      <strong>Collector ID:</strong>{" "}
                      {selectedCollector.collector_id}
                    </div>
                  </div>
                </div>
                <p className="text-sm text-muted-foreground">
                  确定要强制删除此 Collector 吗？此操作不可撤销。
                </p>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setDeleteDialogOpen(false);
                setSelectedCollector(null);
              }}
            >
              取消
            </Button>
            <Button variant="destructive" onClick={handleConfirmDelete}>
              <IconTrash className="h-4 w-4 mr-2" />
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
