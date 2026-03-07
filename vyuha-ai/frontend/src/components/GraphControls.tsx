// ---------------------------------------------------------------------------  // FILTER CONTROLS
// components/GraphControls.tsx — Floating filter controls panel               // FILTER CONTROLS
// SHADCN: replaced checkboxes with Switch, slider with Slider, buttons with Button
// ---------------------------------------------------------------------------  // FILTER CONTROLS

import { useState } from 'react';                                                // FILTER CONTROLS
import { Button } from './ui/button';
import { Switch } from './ui/switch';
import { Slider } from './ui/slider';
import { Badge } from './ui/badge';
import { Separator } from './ui/separator';

// ---------------------------------------------------------------------------  // FILTER CONTROLS
// Public types                                                                 // FILTER CONTROLS
// ---------------------------------------------------------------------------  // FILTER CONTROLS

export interface GraphFilters {                                                   // FILTER CONTROLS
    showNodeTypes:  Set<string>                                                   // FILTER CONTROLS
    showEdgeTypes:  Set<string>                                                   // FILTER CONTROLS
    minImportance:  number                                                        // FILTER CONTROLS
    statusFilter:   'all' | 'failing' | 'healthy' | 'degraded'                   // FILTER CONTROLS
    showDataFlow:   boolean                                                       // FILTER CONTROLS
}                                                                                 // FILTER CONTROLS

export const DEFAULT_FILTERS: GraphFilters = {                                    // FILTER CONTROLS
    showNodeTypes: new Set([                                                      // FILTER CONTROLS
        'service', 'package', 'file', 'function',                                // FILTER CONTROLS
        'struct', 'interface', 'cloud_service', 'data_flow'                      // FILTER CONTROLS
    ]),                                                                           // FILTER CONTROLS
    showEdgeTypes: new Set([                                                      // FILTER CONTROLS
        'contains', 'calls', 'depends_on', 'implements',                         // FILTER CONTROLS
        'imports', 'produces_to', 'consumed_by', 'failed_at'                     // FILTER CONTROLS
    ]),                                                                           // FILTER CONTROLS
    minImportance: 0,                                                             // FILTER CONTROLS
    statusFilter:  'all',                                                         // FILTER CONTROLS
    showDataFlow:  false,                                                         // FILTER CONTROLS
}                                                                                 // FILTER CONTROLS

// ---------------------------------------------------------------------------  // FILTER CONTROLS
// Props                                                                        // FILTER CONTROLS
// ---------------------------------------------------------------------------  // FILTER CONTROLS

interface GraphControlsProps {                                                    // FILTER CONTROLS
    onFilterChange: (filters: GraphFilters) => void                              // FILTER CONTROLS
    stats: {                                                                      // FILTER CONTROLS
        total: number                                                             // FILTER CONTROLS
        visible: number                                                           // FILTER CONTROLS
        failing: number                                                           // FILTER CONTROLS
    }                                                                             // FILTER CONTROLS
}                                                                                 // FILTER CONTROLS

// ---------------------------------------------------------------------------  // FILTER CONTROLS
// Component                                                                    // FILTER CONTROLS
// ---------------------------------------------------------------------------  // FILTER CONTROLS

export const GraphControls = ({                                                   // FILTER CONTROLS
    onFilterChange,                                                               // FILTER CONTROLS
    stats,                                                                        // FILTER CONTROLS
}: GraphControlsProps) => {                                                        // FILTER CONTROLS

    const [open, setOpen] = useState(false)                                       // FILTER CONTROLS
    const [filters, setFilters] = useState<GraphFilters>(                         // FILTER CONTROLS
        DEFAULT_FILTERS)                                                          // FILTER CONTROLS

    const update = (partial: Partial<GraphFilters>) => {                          // FILTER CONTROLS
        const next = { ...filters, ...partial }                                   // FILTER CONTROLS
        setFilters(next)                                                          // FILTER CONTROLS
        onFilterChange(next)                                                      // FILTER CONTROLS
    }                                                                             // FILTER CONTROLS

    const toggleNodeType = (type: string) => {                                    // FILTER CONTROLS
        const next = new Set(filters.showNodeTypes)                               // FILTER CONTROLS
        next.has(type) ? next.delete(type) : next.add(type)                      // FILTER CONTROLS
        update({ showNodeTypes: next })                                           // FILTER CONTROLS
    }                                                                             // FILTER CONTROLS

    const toggleEdgeType = (type: string) => {                                    // FILTER CONTROLS
        const next = new Set(filters.showEdgeTypes)                               // FILTER CONTROLS
        next.has(type) ? next.delete(type) : next.add(type)                      // FILTER CONTROLS
        update({ showEdgeTypes: next })                                           // FILTER CONTROLS
    }                                                                             // FILTER CONTROLS

    return (                                                                      // FILTER CONTROLS
        <div className="absolute top-3 right-3 z-10">                            {/* FILTER CONTROLS */}

            {/* SHADCN: replaced toggle button with <Button variant="outline"> */}
            <Button                                                               // FILTER CONTROLS
                onClick={() => setOpen(o => !o)}                                  // FILTER CONTROLS
                variant="outline"                                                 // SHADCN: outline variant
                size="sm"                                                         // SHADCN: small size
                className="flex items-center gap-2 text-xs"                       // FILTER CONTROLS
            >                                                                     {/* FILTER CONTROLS */}
                <span>⚙ Filters</span>                                           {/* FILTER CONTROLS */}
                {/* SHADCN: replaced failing count span with <Badge> */}
                {stats.failing > 0 && (                                           /* FILTER CONTROLS */
                    <Badge variant="destructive" className="h-4 px-1.5 text-[10px]">
                        {stats.failing} failing                                   {/* FILTER CONTROLS */}
                    </Badge>                                                      /* FILTER CONTROLS */
                )}                                                                {/* FILTER CONTROLS */}
                <span className="text-muted-foreground">                          {/* FILTER CONTROLS */}
                    {stats.visible}/{stats.total}                                 {/* FILTER CONTROLS */}
                </span>                                                           {/* FILTER CONTROLS */}
            </Button>                                                             {/* FILTER CONTROLS */}

            {open && (                                                            /* FILTER CONTROLS */
                <div className="mt-2 bg-black/30 backdrop-blur-xl border                           
                                border-white/[0.08] rounded-lg                          
                                p-3 w-56 space-y-3                                
                                shadow-xl">                                       {/* FILTER CONTROLS */}

                    {/* Quick filters */}                                         {/* FILTER CONTROLS */}
                    <div>                                                          {/* FILTER CONTROLS */}
                        <div className="text-xs text-muted-foreground             
                                        uppercase tracking-wider                  
                                        mb-1.5">                                  {/* FILTER CONTROLS */}
                            Quick Filter                                          {/* FILTER CONTROLS */}
                        </div>                                                    {/* FILTER CONTROLS */}
                        {/* SHADCN: replaced status buttons with <Button variant="outline"> */}
                        <div className="grid grid-cols-2 gap-1">                  {/* FILTER CONTROLS */}
                            {([                                                   // FILTER CONTROLS
                                { value: 'all',      label: 'All' },              // FILTER CONTROLS
                                { value: 'failing',  label: '🔴 Failing' },       // FILTER CONTROLS
                                { value: 'degraded', label: '🟡 Degraded' },      // FILTER CONTROLS
                                { value: 'healthy',  label: '🟢 Healthy' },       // FILTER CONTROLS
                            ] as const).map(opt => (                              // FILTER CONTROLS
                                <Button                                           // FILTER CONTROLS
                                    key={opt.value}                               // FILTER CONTROLS
                                    onClick={() => update({                       // FILTER CONTROLS
                                        statusFilter: opt.value as GraphFilters['statusFilter'] // FILTER CONTROLS
                                    })}                                           // FILTER CONTROLS
                                    variant={filters.statusFilter === opt.value   // SHADCN
                                        ? 'default' : 'outline'}                 // SHADCN
                                    size="sm"                                     // SHADCN
                                    className={`text-xs h-7 ${                    
                                        filters.statusFilter === opt.value        
                                            ? 'bg-blue-900 border-blue-500 text-blue-200 hover:bg-blue-800'
                                            : ''                                  
                                    }`}                                           // FILTER CONTROLS
                                >                                                 {/* FILTER CONTROLS */}
                                    {opt.label}                                   {/* FILTER CONTROLS */}
                                </Button>                                         /* FILTER CONTROLS */
                            ))}                                                   {/* FILTER CONTROLS */}
                        </div>                                                    {/* FILTER CONTROLS */}
                    </div>                                                        {/* FILTER CONTROLS */}

                    <Separator />                                                 {/* SHADCN: section divider */}

                    {/* SHADCN: replaced node type checkboxes with <Switch> */}
                    <div>                                                          {/* FILTER CONTROLS */}
                        <div className="text-xs text-muted-foreground             
                                        uppercase tracking-wider                  
                                        mb-1.5">                                  {/* FILTER CONTROLS */}
                            Node Types                                            {/* FILTER CONTROLS */}
                        </div>                                                    {/* FILTER CONTROLS */}
                        <div className="space-y-1.5">                             {/* FILTER CONTROLS */}
                            {[                                                    // FILTER CONTROLS
                                { type: 'service',       label: 'Services',     color: 'bg-indigo-500' }, // FILTER CONTROLS
                                { type: 'function',      label: 'Functions',    color: 'bg-green-500'  }, // FILTER CONTROLS
                                { type: 'struct',        label: 'Structs',      color: 'bg-purple-500' }, // FILTER CONTROLS
                                { type: 'interface',     label: 'Interfaces',   color: 'bg-pink-500'   }, // FILTER CONTROLS
                                { type: 'cloud_service', label: 'Cloud',        color: 'bg-blue-500'   }, // FILTER CONTROLS
                                { type: 'data_flow',     label: 'Data Flow',    color: 'bg-cyan-500'   }, // FILTER CONTROLS
                            ].map(item => (                                       // FILTER CONTROLS
                                <label                                            // FILTER CONTROLS
                                    key={item.type}                               // FILTER CONTROLS
                                    className="flex items-center                  
                                               gap-2 cursor-pointer"             // FILTER CONTROLS
                                >                                                 {/* FILTER CONTROLS */}
                                    <Switch                                       // SHADCN: replaced checkbox
                                        checked={filters.showNodeTypes.has(item.type)}
                                        onCheckedChange={() => toggleNodeType(item.type)}
                                        className="h-4 w-7 data-[state=checked]:bg-blue-600"
                                    />
                                    <span className={`w-2 h-2 rounded-full ${item.color}`} />
                                    <span className="text-xs text-gray-400">      {/* FILTER CONTROLS */}
                                        {item.label}                              {/* FILTER CONTROLS */}
                                    </span>                                       {/* FILTER CONTROLS */}
                                </label>                                          /* FILTER CONTROLS */
                            ))}                                                   {/* FILTER CONTROLS */}
                        </div>                                                    {/* FILTER CONTROLS */}
                    </div>                                                        {/* FILTER CONTROLS */}

                    <Separator />                                                 {/* SHADCN: section divider */}

                    {/* Edge type toggles (keep colored line indicators) */}
                    <div>                                                          {/* FILTER CONTROLS */}
                        <div className="text-xs text-muted-foreground             
                                        uppercase tracking-wider                  
                                        mb-1.5">                                  {/* FILTER CONTROLS */}
                            Edge Types                                            {/* FILTER CONTROLS */}
                        </div>                                                    {/* FILTER CONTROLS */}
                        <div className="space-y-1.5">                             {/* FILTER CONTROLS */}
                            {[                                                    // FILTER CONTROLS
                                { type: 'calls',       label: 'Calls',       color: '#60A5FA' }, // FILTER CONTROLS
                                { type: 'depends_on',  label: 'Depends On',  color: '#F59E0B' }, // FILTER CONTROLS
                                { type: 'implements',  label: 'Implements',  color: '#A78BFA' }, // FILTER CONTROLS
                                { type: 'produces_to', label: 'Produces To', color: '#34D399' }, // FILTER CONTROLS
                                { type: 'failed_at',   label: 'Failed At',   color: '#EF4444' }, // FILTER CONTROLS
                            ].map(item => (                                       // FILTER CONTROLS
                                <label                                            // FILTER CONTROLS
                                    key={item.type}                               // FILTER CONTROLS
                                    className="flex items-center                  
                                               gap-2 cursor-pointer"             // FILTER CONTROLS
                                    onClick={() => toggleEdgeType(item.type)}     // FILTER CONTROLS
                                >                                                 {/* FILTER CONTROLS */}
                                    <div className="w-4 h-0.5 rounded"            // FILTER CONTROLS
                                         style={{                                 // FILTER CONTROLS
                                             background: filters.showEdgeTypes.has(item.type) // FILTER CONTROLS
                                                 ? item.color                     // FILTER CONTROLS
                                                 : '#374151'                      // FILTER CONTROLS
                                         }}                                       // FILTER CONTROLS
                                    />                                            {/* FILTER CONTROLS */}
                                    <span className="text-xs text-gray-400">      {/* FILTER CONTROLS */}
                                        {item.label}                              {/* FILTER CONTROLS */}
                                    </span>                                       {/* FILTER CONTROLS */}
                                </label>                                          /* FILTER CONTROLS */
                            ))}                                                   {/* FILTER CONTROLS */}
                        </div>                                                    {/* FILTER CONTROLS */}
                    </div>                                                        {/* FILTER CONTROLS */}

                    <Separator />                                                 {/* SHADCN: section divider */}

                    {/* SHADCN: replaced <input type="range"> with <Slider> */}
                    <div>                                                          {/* FILTER CONTROLS */}
                        <div className="text-xs text-muted-foreground             
                                        uppercase tracking-wider                  
                                        mb-1.5">                                  {/* FILTER CONTROLS */}
                            Min Edge Importance: {filters.minImportance}           {/* FILTER CONTROLS */}
                        </div>                                                    {/* FILTER CONTROLS */}
                        <Slider                                                   // SHADCN: replaced range input
                            min={0}                                               // FILTER CONTROLS
                            max={80}                                              // FILTER CONTROLS
                            step={10}                                             // FILTER CONTROLS
                            value={[filters.minImportance]}                       // FILTER CONTROLS
                            onValueChange={(v: number[]) => update({                        // FILTER CONTROLS
                                minImportance: v[0]                               // FILTER CONTROLS
                            })}                                                   // FILTER CONTROLS
                            className="w-full"                                    // FILTER CONTROLS
                        />                                                        {/* FILTER CONTROLS */}
                        <div className="flex justify-between                      
                                        text-xs text-gray-600 mt-1">             {/* FILTER CONTROLS */}
                            <span>All</span>                                      {/* FILTER CONTROLS */}
                            <span>Critical only</span>                            {/* FILTER CONTROLS */}
                        </div>                                                    {/* FILTER CONTROLS */}
                    </div>                                                        {/* FILTER CONTROLS */}

                    {/* SHADCN: replaced reset button with <Button variant="ghost"> */}
                    <Button                                                       // FILTER CONTROLS
                        onClick={() => {                                          // FILTER CONTROLS
                            setFilters(DEFAULT_FILTERS)                           // FILTER CONTROLS
                            onFilterChange(DEFAULT_FILTERS)                       // FILTER CONTROLS
                        }}                                                        // FILTER CONTROLS
                        variant="ghost"                                           // SHADCN: ghost variant
                        size="sm"                                                 // SHADCN: small size
                        className="w-full text-xs text-muted-foreground"          // FILTER CONTROLS
                    >                                                             {/* FILTER CONTROLS */}
                        Reset Filters                                             {/* FILTER CONTROLS */}
                    </Button>                                                     {/* FILTER CONTROLS */}
                </div>                                                            /* FILTER CONTROLS */
            )}                                                                    {/* FILTER CONTROLS */}
        </div>                                                                    /* FILTER CONTROLS */
    )                                                                             // FILTER CONTROLS
}                                                                                 // FILTER CONTROLS
