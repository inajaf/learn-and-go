//This is a runnable demo application.
//Run: go run ./09_complete_demo/cmd/main.go
//
//You will see how the whole system works together:
//- OrderService accepts the order
//- InventoryService checks the warehouse (synchronously - analogous to gRPC)
//- EventBus sends out an event (asynchronously - analogous to Kafka)
//- NotificationService sends email
//- AnalyticsService records statistics
package main

import (
	"context"
	"fmt"

	demo "learning_path/09_complete_demo"
)

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("🚀 Go Microservices - Full system demo")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	//─── System initialization ─────────────────────────────────
	fmt.Println("🔧 Initializing the system...")
	system := demo.NewSystem(map[string]int{
		"iphone-15":   50,
		"macbook-air": 20,
		"airpods-pro": 100,
		"ipad-mini":   30,
	})
	fmt.Println("✓ OrderService is ready")
	fmt.Println("✓ InventoryService ready (warehouse: iPhone×50, MacBook×20, AirPods×100, iPad×30)")
	fmt.Println("✓ EventBus is ready (similar to Kafka)")
	fmt.Println("✓ NotificationService is subscribed to order.created, order.cancelled")
	fmt.Println("✓ AnalyticsService is subscribed to order.created, order.cancelled")
	fmt.Println()

	//─── Creating the first order ───────────────────────────────
	fmt.Println("📦 Create order #1 (Alice buys iPhone + AirPods)...")
	req1 := demo.CreateOrderRequest{
		CustomerID: "customer-alice",
		Items: []demo.CreateItemRequest{
			{ProductID: "iphone-15", Name: "iPhone 15 Pro", Quantity: 1, UnitPrice: 99900},
			{ProductID: "airpods-pro", Name: "AirPods Pro", Quantity: 2, UnitPrice: 24900},
		},
	}

	ctx := context.Background()

	resp1, err := system.OrderSvc.CreateOrder(ctx, req1)
	if err != nil {
		fmt.Printf("✗ Error: %v\\n", err)
	} else {
		fmt.Printf("✓ Order created: %s\\n", resp1.ID)
		fmt.Printf("Status: %s\\n", resp1.Status)
		fmt.Printf("Amount: %.2f rub.\\n", resp1.TotalAmount)
		fmt.Printf("Created by: %s\\n", resp1.CreatedAt)
	}
	fmt.Println()

	//─── Creating a second order ───────────────────────────────
	fmt.Println("📦 Create order #2 (Bob buys a MacBook)...")
	req2 := demo.CreateOrderRequest{
		CustomerID: "customer-bob",
		Items: []demo.CreateItemRequest{
			{ProductID: "macbook-air", Name: "MacBook Air M3", Quantity: 1, UnitPrice: 149900},
		},
	}

	resp2, err := system.OrderSvc.CreateOrder(ctx, req2)
	if err != nil {
		fmt.Printf("✗ Error: %v\\n", err)
	} else {
		fmt.Printf("✓ Order created: %s\\n", resp2.ID)
		fmt.Printf("Status: %s\\n", resp2.Status)
		fmt.Printf("Amount: %.2f rub.\\n", resp2.TotalAmount)
	}
	fmt.Println()

	//─── Trying to order more than there is ─────────────────────
	fmt.Println("📦 We create order #3 (Greedy Petya wants 1000 MacBooks)...")
	req3 := demo.CreateOrderRequest{
		CustomerID: "customer-pete",
		Items: []demo.CreateItemRequest{
			{ProductID: "macbook-air", Name: "MacBook Air", Quantity: 1000, UnitPrice: 149900},
		},
	}

	_, err = system.OrderSvc.CreateOrder(ctx, req3)
	if err != nil {
		fmt.Printf("✗ Expected error: %v\\n", err)
		fmt.Println("👆 Saga compensation: warehouse has not changed")
	}
	fmt.Println()

	//─── Cancellation of the first order ─────────────────────────────────
	if resp1.ID != "" {
		fmt.Printf("🚫 We are canceling order #1 (%s)...\\n", resp1.ID)
		cancelled, err := system.OrderSvc.CancelOrder(ctx, resp1.ID)
		if err != nil {
			fmt.Printf("✗ Cancellation error: %v\\n", err)
		} else {
			fmt.Printf("✓ Order cancelled, status: %s\\n", cancelled.Status)
		}
		fmt.Println()
	}

	//─── Final state of the system ───────────────────────────
	fmt.Println(system.PrintStatus())

	//─── Log of warehouse operations ──────────────────────────────
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("WAREHOUSE OPERATIONS LOG (Sync/gRPC)")
	fmt.Println("═══════════════════════════════════════")
	for _, entry := range system.Inventory.Log() {
		fmt.Printf("  %s\n", entry)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("✅ Demo completed!")
	fmt.Println()
	fmt.Println("📚 What did you just see:")
	fmt.Println("  ┌─────────────────────────────────────────────────────┐")
	fmt.Println("│ 1. Interfaces (Module 1) - DI, loose coupling │")
	fmt.Println("│ 2. DTO + mapping (Module 2) - data layers │")
	fmt.Println("│ 3. Sync call (Module 3) - warehouse verified via DI │")
	fmt.Println("│ 4. Events (Module 4) - events are sent async │")
	fmt.Println("│ 5. Saga (Module 8) - error compensation │")
	fmt.Println("│ - Notif. and Analytics do not know about OrderService │")
	fmt.Println("│ - OrderService does not know about Notif. and Analytics │")
	fmt.Println("  └─────────────────────────────────────────────────────┘")
	fmt.Println("═══════════════════════════════════════════════════════════")
}
