<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiScheduledJobsByIdFire extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id Scheduled job ID
     * @param null|\FlowCatalyst\Generated\Model\FireRequest $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\FireRequest $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/scheduled-jobs/{id}/fire');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\FireRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PostApiScheduledJobsByIdFireNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiScheduledJobsByIdFireConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\CreatedResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (202 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\CreatedResponse', 'json');
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiScheduledJobsByIdFireNotFoundException($response);
        }
        if (409 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiScheduledJobsByIdFireConflictException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearer_auth'];
    }
}