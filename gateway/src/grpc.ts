import * as grpc from '@grpc/grpc-js';
import * as protoLoader from '@grpc/proto-loader';
import path from 'path';

const PROTO_PATH = path.resolve(__dirname, '../../shared/proto/agent.proto');

const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
	keepCase: true,
	longs: String,
	enums: String,
	defaults: true,
	oneofs: true,
});

const protoDescriptor = grpc.loadPackageDefinition(packageDefinition) as any;
const agentProto = protoDescriptor.agent;

// Create gRPC client connecting to FastAPI AI Service on port 50051
export const grpcClient = new agentProto.AgentService(
	process.env.AI_SERVICE_URL || 'ai-service:50051',
	grpc.credentials.createInsecure()
);

export interface ChatRpcRequest {
	message: string;
	session_id: string;
	server_id: number;
}

export interface ChatRpcResponse {
	response_text: string;
	is_approval_required: boolean;
	proposed_tool_calls: {
		name: string;
		arguments_json: string;
		description: string;
	}[];
}

// Promisified Chat call wrapper
export function sendAgentChat(req: ChatRpcRequest): Promise<ChatRpcResponse> {
	return new Promise((resolve, reject) => {
		grpcClient.Chat(req, (err: any, response: ChatRpcResponse) => {
			if (err) {
				return reject(err);
			}
			resolve(response);
		});
	});
}
