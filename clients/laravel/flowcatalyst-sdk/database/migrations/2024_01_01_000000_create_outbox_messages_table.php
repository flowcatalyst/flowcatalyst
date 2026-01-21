<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        Schema::create('outbox_messages', function (Blueprint $table) {
            // Primary key - TSID as Crockford Base32 string
            $table->string('id', 255)->primary();

            // Tenant and partition for routing
            $table->bigInteger('tenant_id');
            $table->string('partition_id', 100);

            // Message type: EVENT or DISPATCH_JOB
            $table->string('type', 20);

            // Payload
            $table->longText('payload');
            $table->bigInteger('payload_size');

            // Processing status
            $table->string('status', 20)->default('PENDING');
            $table->timestamp('created_at')->useCurrent();
            $table->timestamp('processed_at')->nullable();

            // Retry handling
            $table->integer('retry_count')->default(0);
            $table->text('error_reason')->nullable();

            // Optional headers (JSON)
            $table->json('headers')->nullable();

            // Indexes for efficient polling
            // Primary polling query: get pending messages for a partition
            $table->index(
                ['tenant_id', 'partition_id', 'status', 'created_at'],
                'idx_outbox_pending'
            );

            // Status-based queries
            $table->index(
                ['status', 'created_at'],
                'idx_outbox_status'
            );

            // Discovery query for active partitions
            $table->index(
                ['created_at'],
                'idx_outbox_created'
            );
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('outbox_messages');
    }
};
