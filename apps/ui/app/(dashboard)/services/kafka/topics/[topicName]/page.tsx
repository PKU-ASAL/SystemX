"use client";

import React from "react";
import { KafkaTopicDetail } from "@/components/kafka/kafka-topic-detail";
import { useRouter } from "next/navigation";

interface TopicDetailPageProps {
  params: Promise<{
    topicName: string;
  }>;
}

export default function TopicDetailPage({ params }: TopicDetailPageProps) {
  const router = useRouter();
  const resolvedParams = React.use(params);

  const handleBack = () => {
    router.push("/kafka/topics");
  };

  // URL decode the topic name in case it contains special characters
  const decodedTopicName = decodeURIComponent(resolvedParams.topicName);

  return <KafkaTopicDetail topicName={decodedTopicName} onBack={handleBack} />;
}
