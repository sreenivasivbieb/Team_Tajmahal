// ---------------------------------------------------------------------------  // BUNDLE
// components/edges/BundledEdge.tsx — Custom edge for high fan-out bundles     // BUNDLE
// ---------------------------------------------------------------------------  // BUNDLE

import { useState, type FC } from 'react';                                       // BUNDLE
import { getBezierPath, type EdgeProps } from 'reactflow';                        // BUNDLE

/** Data payload attached to each bundled edge by `bundleHighFanoutEdges()`. */  // BUNDLE
interface BundledEdgeData {                                                        // BUNDLE
  type: 'bundled';                                                                // BUNDLE
  count: number;                                                                  // BUNDLE
  originalEdges: unknown[];                                                       // BUNDLE
}                                                                                 // BUNDLE

const BundledEdge: FC<EdgeProps<BundledEdgeData>> = ({                            // BUNDLE
  id,                                                                             // BUNDLE
  sourceX,                                                                        // BUNDLE
  sourceY,                                                                        // BUNDLE
  targetX,                                                                        // BUNDLE
  targetY,                                                                        // BUNDLE
  sourcePosition,                                                                 // BUNDLE
  targetPosition,                                                                 // BUNDLE
  data,                                                                           // BUNDLE
  style = {},                                                                     // BUNDLE
}) => {                                                                           // BUNDLE
  const [expanded, setExpanded] = useState(false);                                // BUNDLE

  const [edgePath, labelX, labelY] = getBezierPath({                              // BUNDLE
    sourceX,                                                                      // BUNDLE
    sourceY,                                                                      // BUNDLE
    targetX,                                                                      // BUNDLE
    targetY,                                                                      // BUNDLE
    sourcePosition,                                                               // BUNDLE
    targetPosition,                                                               // BUNDLE
  });                                                                             // BUNDLE

  const count = data?.count ?? 0;                                                 // BUNDLE

  return (                                                                        // BUNDLE
    <>                                                                            {/* BUNDLE */}
      {/* Thicker background stroke to convey "bundle" */}                        {/* BUNDLE */}
      <path                                                                       // BUNDLE
        id={id}                                                                   // BUNDLE
        className="react-flow__edge-path"                                         // BUNDLE
        d={edgePath}                                                              // BUNDLE
        style={{                                                                  // BUNDLE
          ...style,                                                               // BUNDLE
          stroke: '#6366f1',                                                      // BUNDLE
          strokeWidth: Math.min(2 + count * 0.5, 8),                              // BUNDLE
          strokeDasharray: '6 3',                                                 // BUNDLE
        }}                                                                        // BUNDLE
      />                                                                          {/* BUNDLE */}

      {/* Label pill with expand / hide toggle */}                                {/* BUNDLE */}
      <foreignObject                                                              // BUNDLE
        width={80}                                                                // BUNDLE
        height={32}                                                               // BUNDLE
        x={labelX - 40}                                                           // BUNDLE
        y={labelY - 16}                                                           // BUNDLE
        requiredExtensions="http://www.w3.org/1999/xhtml"                         // BUNDLE
      >                                                                           {/* BUNDLE */}
        <button                                                                   // BUNDLE
          onClick={(e) => {                                                       // BUNDLE
            e.stopPropagation();                                                  // BUNDLE
            setExpanded(prev => !prev);                                           // BUNDLE
          }}                                                                      // BUNDLE
          style={{                                                                // BUNDLE
            width: '100%',                                                        // BUNDLE
            height: '100%',                                                       // BUNDLE
            display: 'flex',                                                      // BUNDLE
            alignItems: 'center',                                                 // BUNDLE
            justifyContent: 'center',                                             // BUNDLE
            background: '#312e81',                                                // BUNDLE
            color: '#c7d2fe',                                                     // BUNDLE
            border: '1px solid #6366f1',                                          // BUNDLE
            borderRadius: 6,                                                      // BUNDLE
            fontSize: 10,                                                         // BUNDLE
            cursor: 'pointer',                                                    // BUNDLE
            whiteSpace: 'nowrap',                                                 // BUNDLE
          }}                                                                      // BUNDLE
          title={expanded ? 'Hide individual edges' : 'Show individual edges'}    // BUNDLE
        >                                                                         {/* BUNDLE */}
          {expanded ? 'Hide' : `${count} edges`}                                  {/* BUNDLE */}
        </button>                                                                 {/* BUNDLE */}
      </foreignObject>                                                            {/* BUNDLE */}
    </>                                                                           // BUNDLE
  );                                                                              // BUNDLE
};                                                                                // BUNDLE

export default BundledEdge;                                                       // BUNDLE
