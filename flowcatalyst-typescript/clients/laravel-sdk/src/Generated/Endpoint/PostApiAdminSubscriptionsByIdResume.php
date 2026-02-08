<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiAdminSubscriptionsByIdResume extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * Re-enable dispatch job creation
     * @param string $id
     */
    public function __construct(string $id)
    {
        $this->id = $id;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/admin/subscriptions/{id}/resume');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeForbiddenException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return json_decode($body);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeNotFoundException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}