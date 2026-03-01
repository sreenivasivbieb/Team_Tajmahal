/**
 * Maps backend GraphNode.type values to ReactFlow custom node type keys.
 * These keys must match the keys registered in GraphCanvas's nodeTypes object.
 */
export function nodeTypeToRF(type: string): string {
  switch (type) {
    case 'service':
      return 'serviceNode';
    case 'function':
      return 'functionNode';
    case 'cloud_service':
      return 'cloudNode';
    case 'data_flow':
      return 'dataFlowNode';
    case 'package':
      return 'packageNode';
    case 'file':
      return 'fileNode';
    case 'struct':
      return 'structNode';
    case 'interface':
      return 'interfaceNode';
    case 'repository':
      return 'repositoryNode';
    case 'runtime_instance':
      return 'runtimeNode';
    default:
      return 'default';
  }
}
