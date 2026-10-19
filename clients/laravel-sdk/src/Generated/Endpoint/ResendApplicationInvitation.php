<?php

namespace FlowCatalyst\Generated\Endpoint;

class ResendApplicationInvitation extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $id;
    /**
     * @param string $appCode Application code
     * @param string $id The invited user's id
     * @param null|\FlowCatalyst\Generated\Model\ResendInvitationRequest $requestBody
     */
    public function __construct(string $appCode, string $id, ?\FlowCatalyst\Generated\Model\ResendInvitationRequest $requestBody = null)
    {
        $this->appCode = $appCode;
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
        return str_replace(['{appCode}', '{id}'], [$this->appCode, $this->id], '/api/applications/{appCode}/invitations/{id}/resend');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ResendInvitationRequest) {
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
     *
     * @return null|\FlowCatalyst\Generated\Model\ResendInvitationResponse|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ResendInvitationResponse', 'json');
        }
        if (mb_strpos(strtolower($contentType), 'application/json') !== false) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}