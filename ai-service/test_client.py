import grpc
import agent_pb2
import agent_pb2_grpc
import sys

def test_chat():
    channel = grpc.insecure_channel('127.0.0.1:50051')
    stub = agent_pb2_grpc.AgentServiceStub(channel)
    
    print("Testing client started. Sending message: 'list containers'...")
    try:
        response = stub.Chat(agent_pb2.ChatRequest(
            message="list containers",
            session_id="test_session_123",
            server_id=1
        ))
        
        print("\n--- gRPC Response ---")
        print(f"Response Text: {response.response_text}")
        print(f"Approval Required: {response.is_approval_required}")
        print("Proposed Tool Calls:")
        for tc in response.proposed_tool_calls:
            print(f"  - Name: {tc.name}")
            print(f"    Arguments: {tc.arguments_json}")
            print(f"    Description: {tc.description}")
            
        if response.is_approval_required and response.proposed_tool_calls:
            # Simulate approval feeding back
            print("\nSimulating approval. Sending tool result back...")
            tool_result_msg = f"Approved. Tool Result: [{{\"id\": \"123\", \"name\": \"web-server\", \"status\": \"running\"}}]"
            
            response2 = stub.Chat(agent_pb2.ChatRequest(
                message=tool_result_msg,
                session_id="test_session_123",
                server_id=1
            ))
            
            print("\n--- gRPC Response 2 ---")
            print(f"Response Text: {response2.response_text}")
            print(f"Approval Required: {response2.is_approval_required}")
            
    except grpc.RpcError as e:
        print(f"gRPC call failed: {e.code()} - {e.details()}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    test_chat()
